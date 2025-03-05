// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	"github.com/mholt/archives"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
)

// Publish publishes a bundle to a remote OCI registry
func (b *Bundle) Publish() error {
	b.cfg.PublishOpts.Destination = boci.EnsureOCIPrefix(b.cfg.PublishOpts.Destination)

	// load bundle metadata into memory
	// todo: having the tmp dir be the provider.dst is weird
	provider, err := NewBundleProvider(b.cfg.PublishOpts.Source, b.tmp)
	if err != nil {
		return err
	}
	filepaths, err := provider.LoadBundleMetadata()
	if err != nil {
		return err
	}
	if err := utils.ReadYAMLStrict(filepaths[config.BundleYAML], &b.bundle); err != nil {
		return err
	}
	err = os.RemoveAll(filepath.Join(b.tmp, "blobs")) // clear tmp dir
	if err != nil {
		return err
	}

	// construct a fileHandler that extracts all files from the archive with the same relative pathing
	fileHandler := func(_ context.Context, file archives.FileInfo) error {
		extractPath := filepath.Join(b.tmp, file.NameInArchive)

		if file.IsDir() {
			return os.MkdirAll(extractPath, 0744)
		}

		if err = os.MkdirAll(filepath.Dir(extractPath), 0744); err != nil {
			return err
		}

		stream, err := file.Open()
		if err != nil {
			return err
		}
		defer stream.Close()

		fileBytes, err := io.ReadAll(stream)
		if err != nil {
			return err
		}

		return os.WriteFile(extractPath, fileBytes, 0644)
	}

	bundleBytes, err := os.ReadFile(b.cfg.PublishOpts.Source)
	if err != nil {
		return err
	}

	err = config.BundleArchiveFormat.Extract(context.TODO(), bytes.NewReader(bundleBytes), fileHandler)
	if err != nil {
		return err
	}

	// create new OCI artifact in remote
	ociURL := b.cfg.PublishOpts.Destination
	bundleName := b.bundle.Metadata.Name

	// tag bundle with metadata.version, unless user specifies a version
	bundleTag := b.bundle.Metadata.Version
	if b.cfg.PublishOpts.Version != "" {
		bundleTag = b.cfg.PublishOpts.Version
	}

	platform := ocispec.Platform{
		Architecture: config.GetArch(),
		OS:           oci.MultiOS,
	}
	remote, err := zoci.NewRemote(context.TODO(), fmt.Sprintf("%s/%s:%s", ociURL, bundleName, bundleTag), platform)
	if err != nil {
		return err
	}
	err = provider.PublishBundle(b.bundle, remote.OrasRemote)
	if err != nil {
		return err
	}
	return nil
}
