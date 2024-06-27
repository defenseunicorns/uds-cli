// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/layout"
	"github.com/defenseunicorns/zarf/src/pkg/packager/filters"
	zarfSources "github.com/defenseunicorns/zarf/src/pkg/packager/sources"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pterm/pterm"
)

// Inspect pulls/unpacks a bundle's metadata and shows it
func (b *Bundle) Inspect() error {
	//  handle --list-images flag
	if b.cfg.InspectOpts.ListImages {
		err := b.listImages()
		if err != nil {
			return err
		}
		return nil
	}

	// Check that provided oci source path is valid, and update it if it's missing the full path
	source, err := CheckOCISourcePath(b.cfg.InspectOpts.Source)
	if err != nil {
		return fmt.Errorf("source %s is either invalid or doesn't exist", b.cfg.InspectOpts.Source)
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

	return nil
}

func (b *Bundle) listImages() error {
	if err := utils.CheckYAMLSourcePath(b.cfg.InspectOpts.Source); err != nil {
		return err
	}

	if err := utils.ReadYAMLStrict(b.cfg.InspectOpts.Source, &b.bundle); err != nil {
		return err
	}

	// find images in the packages taking into account optional components
	imgs, err := b.getPackageImages()
	if err != nil {
		return err
	}

	formattedImgs := strings.Join(imgs, "\n")

	// print to stdout to enable users to easily grab the output
	// (and stderr for backwards compatibility)
	pterm.SetDefaultOutput(io.MultiWriter(os.Stderr, os.Stdout))
	pterm.Printfln("%s\n", formattedImgs)
	return nil
}

func (b *Bundle) getPackageImages() ([]string, error) {
	// use a map to track the images for easy de-duping
	imgMap := make(map[string]string)

	for _, pkg := range b.bundle.Packages {
		// get package source
		source, err := b.getSource(pkg)
		if err != nil {
			return nil, err
		}

		tmpDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
		if err != nil {
			return nil, err
		}
		pkgPaths := layout.New(tmpDir)
		zarfPkg, _, err := source.LoadPackageMetadata(context.TODO(), pkgPaths, false, true)
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

	// convert img map to list of strings
	var images []string
	for _, img := range imgMap {
		images = append(images, img)
	}

	return images, nil
}

func (b *Bundle) getSource(pkg types.Package) (zarfSources.PackageSource, error) {
	var source zarfSources.PackageSource
	if pkg.Repository != "" {
		// handle remote packages
		url := fmt.Sprintf("oci://%s:%s", pkg.Repository, pkg.Ref)
		platform := ocispec.Platform{
			Architecture: config.GetArch(),
			OS:           oci.MultiOS,
		}
		remote, err := zoci.NewRemote(url, platform)
		if err != nil {
			return nil, err
		}

		source = &zarfSources.OCISource{
			ZarfPackageOptions: &zarfTypes.ZarfPackageOptions{},
			Remote:             remote,
		}
	} else if pkg.Path != "" {
		// handle local packages
		err := os.Chdir(filepath.Dir(b.cfg.InspectOpts.Source)) // change to the bundle's directory
		if err != nil {
			return nil, err
		}

		bundleArch := config.GetArch(b.bundle.Metadata.Architecture)
		tarballName := fmt.Sprintf("zarf-package-%s-%s-%s.tar.zst", pkg.Name, bundleArch, pkg.Ref)
		source = &zarfSources.TarballSource{
			ZarfPackageOptions: &zarfTypes.ZarfPackageOptions{
				PackageSource: filepath.Join(pkg.Path, tarballName),
			},
		}
	} else {
		return nil, fmt.Errorf("package %s is missing a repository or path", pkg.Name)
	}
	return source, nil
}
