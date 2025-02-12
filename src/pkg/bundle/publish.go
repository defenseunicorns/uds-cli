// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/tfparser"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	av3 "github.com/mholt/archiver/v3"
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

	_, err = os.Stat(filepaths[config.BundleTF])
	if err == nil {
		// Parse the 'uds-bundle.tf' file into the bundle struct
		if err := tfparser.ParseBundle(filepaths[config.BundleTF], filepaths[config.BundleTFConfig], &b.bundle); err != nil {
			return err
		}
	} else {
		// Parse the 'uds-bundle.yaml' file into the bundle struct
		if err := utils.ReadYAMLStrict(filepaths[config.BundleYAML], &b.bundle); err != nil {
			return err
		}
	}
	err = os.RemoveAll(filepath.Join(b.tmp, "blobs")) // clear tmp dir
	if err != nil {
		return err
	}

	// unarchive bundle into empty tmp dir
	err = av3.Unarchive(b.cfg.PublishOpts.Source, b.tmp) // todo: awkward to use old version of mholt/archiver
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
