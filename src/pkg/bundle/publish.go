// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	oci "github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	av3 "github.com/mholt/archiver/v3"

	"github.com/defenseunicorns/uds-cli/src/config"
)

// Publish publishes a bundle to a remote OCI registry
func (b *Bundler) Publish() error {
	// load bundle metadata into memory
	provider, err := NewBundleProvider(context.TODO(), b.cfg.PublishOpts.Source, b.tmp)
	if err != nil {
		return err
	}
	loaded, err := provider.LoadBundleMetadata()
	if err != nil {
		return err
	}
	if err := utils.ReadYaml(loaded[config.BundleYAML], &b.bundle); err != nil {
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
	bundleTag := b.bundle.Metadata.Version
	bundleArch := b.bundle.Metadata.Architecture
	remote, err := oci.NewOrasRemote(fmt.Sprintf("%s/%s:%s-%s", ociURL, bundleName, bundleTag, bundleArch))
	if err != nil {
		return err
	}
	err = provider.PublishBundle(b.bundle, remote)
	if err != nil {
		return err
	}
	return nil
}
