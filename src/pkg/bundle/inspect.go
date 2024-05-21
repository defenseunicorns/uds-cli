// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
)

// Inspect pulls/unpacks a bundle's metadata and shows it
func (b *Bundle) Inspect() error {

	// Check that provided oci source path is valid, and update it if it's missing the full path
	source, err := CheckOCISourcePath(b.cfg.InspectOpts.Source)
	if err != nil {
		return err
	}
	b.cfg.InspectOpts.Source = source

	// create a new provider
	provider, err := NewBundleProvider(b.cfg.InspectOpts.Source, b.tmp)
	if err != nil {
		return err
	}

	// pull the bundle's metadata + sig + sboms (optional)
	loaded, err := provider.LoadBundleMetadata()
	if err != nil {
		return err
	}

	// validate the sig (if present)
	if err := ValidateBundleSignature(loaded[config.BundleYAML], loaded[config.BundleYAMLSignature], b.cfg.InspectOpts.PublicKeyPath); err != nil {
		return err
	}

	// pull sbom
	if b.cfg.InspectOpts.IncludeSBOM {
		err := provider.CreateBundleSBOM(b.cfg.InspectOpts.ExtractSBOM)
		if err != nil {
			return err
		}
	}
	// read the bundle's metadata into memory
	if err := utils.ReadYAMLStrict(loaded[config.BundleYAML], &b.bundle); err != nil {
		return err
	}

	// show the bundle's metadata
	zarfUtils.ColorPrintYAML(b.bundle, nil, false)

	// TODO: showing package metadata?
	// TODO: could be cool to have an interactive mode that lets you select a package and show its metadata
	return nil
}
