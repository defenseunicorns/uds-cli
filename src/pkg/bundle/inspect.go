// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/layout"
	"github.com/defenseunicorns/zarf/src/pkg/packager/filters"
	zarfSources "github.com/defenseunicorns/zarf/src/pkg/packager/sources"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Inspect pulls/unpacks a bundle's metadata and shows it
func (b *Bundle) Inspect() error {
	// Check if the source is a YAML file
	if b.cfg.InspectOpts.ListImages && filepath.Ext(b.cfg.InspectOpts.Source) == ".yaml" { // todo: or .yml
		source, err := CheckYAMLSourcePath(b.cfg.InspectOpts.Source)
		if err != nil {
			return err
		}
		b.cfg.InspectOpts.Source = source

		// read the bundle's metadata into memory
		if err := utils.ReadYAMLStrict(b.cfg.InspectOpts.Source, &b.bundle); err != nil {
			return err
		}

		imgs, err := b.extractImagesFromPackages()
		if err != nil {
			return err
		}
		fmt.Println(strings.Join(imgs, "\n"))
		return nil
	}

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

func (b *Bundle) extractImagesFromPackages() ([]string, error) {
	imgMap := make(map[string]string)
	for _, pkg := range b.bundle.Packages {
		if pkg.Repository != "" && pkg.Ref != "" {

			url := fmt.Sprintf("oci://%s:%s", pkg.Repository, pkg.Ref)
			platform := ocispec.Platform{
				Architecture: config.GetArch(),
				OS:           oci.MultiOS,
			}
			remote, err := zoci.NewRemote(url, platform)
			if err != nil {
				return nil, err
			}

			source := zarfSources.OCISource{
				ZarfPackageOptions: &zarfTypes.ZarfPackageOptions{
					PublicKeyPath: "",
				},
				Remote: remote,
			}

			tmpDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
			if err != nil {
				return nil, err
			}
			pkgPaths := layout.New(tmpDir)
			zarfPkg, _, err := source.LoadPackageMetadata(pkgPaths, false, true)
			if err != nil {
				return nil, err
			}

			// create filter for optional components
			inspectFilter := filters.Combine(
				filters.ForDeploy(strings.Join(pkg.OptionalComponents, ","), false),
			)

			filteredComponents, err := inspectFilter.Apply(zarfPkg)
			if err != nil {
				return nil, err
			}

			// grab images from each filtered component
			for _, component := range filteredComponents {
				for _, img := range component.Images {
					imgMap[img] = img
				}
			}
		}
	}

	// convert img map to list of strings
	var images []string
	for _, img := range imgMap {
		images = append(images, img)
	}

	return images, nil
}
