// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
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
	if err := utils.ReadYAMLStrict(filepaths[config.BundleYAML], &b.bundle); err != nil {
		return err
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

	// tag bundle with version, unless user specifies a tag
	bundleTag := b.bundle.Metadata.Version
	if b.cfg.PublishOpts.Tag != "" {
		bundleTag = b.cfg.PublishOpts.Tag
	}

	platform := ocispec.Platform{
		Architecture: config.GetArch(),
		OS:           oci.MultiOS,
	}
	remote, err := zoci.NewRemote(fmt.Sprintf("%s/%s:%s", ociURL, bundleName, bundleTag), platform)
	if err != nil {
		return err
	}
	err = provider.PublishBundle(b.bundle, remote.OrasRemote)
	if err != nil {
		return err
	}
	return nil
}
