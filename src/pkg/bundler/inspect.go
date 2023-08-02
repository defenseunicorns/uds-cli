// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler contains functions for interacting with, managing and deploying UDS bundles.
package bundler

import (
	"context"

	"github.com/defenseunicorns/zarf/src/pkg/utils"
	// ocistore "oras.land/oras-go/v2/content/oci"
)

// Inspect pulls/unpacks a bundle's metadata and shows it
func (b *Bundler) Inspect() error {
	ctx := context.TODO()
	// create a new provider
	provider, err := NewProvider(ctx, b.cfg.InspectOpts.Source, b.tmp)
	if err != nil {
		return err
	}

	// pull the bundle's metadata + sig
	loaded, err := provider.LoadBundleMetadata()
	if err != nil {
		return err
	}

	// validate the sig (if present)
	if err := ValidateBundleSignature(loaded[BundleYAML], loaded[BundleYAMLSignature], b.cfg.InspectOpts.PublicKeyPath); err != nil {
		return err
	}

	// read the bundle's metadata into memory
	if err := utils.ReadYaml(loaded[BundleYAML], &b.bundle); err != nil {
		return err
	}

	// show the bundle's metadata
	utils.ColorPrintYAML(b.bundle, nil, false)

	// TODO: showing SBOMs?
	// TODO: showing package metadata?
	// TODO: could be cool to have an interactive mode that lets you select a package and show its metadata
	return nil
}
