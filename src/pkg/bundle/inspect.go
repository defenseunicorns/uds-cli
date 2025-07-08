// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	goyaml "github.com/goccy/go-yaml"
	"github.com/pterm/pterm"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
)

// Inspect pulls/unpacks a bundle's metadata and shows it
func (b *Bundle) Inspect() error {
	// print to stdout to enable users to easily grab the output
	pterm.SetDefaultOutput(os.Stdout)
	var warns []string

	if err := utils.CheckYAMLSourcePath(b.cfg.InspectOpts.Source); err == nil {
		b.cfg.InspectOpts.IsYAMLFile = true
		if err := utils.ReadYAMLStrict(b.cfg.InspectOpts.Source, &b.bundle); err != nil {
			return err
		}
	} else {
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
		filepaths, err := provider.LoadBundleMetadata()
		if err != nil {
			return err
		}

		// validate the sig (if present)
		if err := ValidateBundleSignature(filepaths[config.BundleYAML], filepaths[config.BundleYAMLSignature], b.cfg.InspectOpts.PublicKeyPath); err != nil {
			return err
		}

		// read the bundle's metadata into memory
		if err := utils.ReadYAMLStrict(filepaths[config.BundleYAML], &b.bundle); err != nil {
			return err
		}

		// pull sbom
		if b.cfg.InspectOpts.IncludeSBOM {
			warns, err = provider.CreateBundleSBOM(b.cfg.InspectOpts.ExtractSBOM, b.bundle.Metadata.Name)
			if err != nil {
				return err
			}
		}
	}

	// handle --list-variables flag
	if b.cfg.InspectOpts.ListVariables {
		err := b.listVariables()
		if err != nil {
			return err
		}
		return nil
	}

	//  handle --list-images flag
	if b.cfg.InspectOpts.ListImages {
		err := b.listImages()
		if err != nil {
			return err
		}
		return nil
	}

	if err := zarfUtils.ColorPrintYAML(b.bundle, nil, false); err != nil {
		message.Warn("error printing bundle yaml")
	}

	// print warnings to stderr
	pterm.SetDefaultOutput(os.Stderr)
	for _, warn := range warns {
		message.Warn(warn)
	}

	return nil
}

func (b *Bundle) listImages() error {
	// find images in the packages taking into account optional components
	pkgImgMap := make(map[string][]string)

	for _, pkg := range b.bundle.Packages {
		pkgImgMap[pkg.Name] = make([]string, 0)

		zarfPkg, err := b.getMetadata(pkg)
		if err != nil {
			return err
		}

		// create filter for optional components
		inspectFilter := filters.Combine(
			filters.ForDeploy(strings.Join(pkg.OptionalComponents, ","), false),
		)

		filteredComponents, err := inspectFilter.Apply(zarfPkg)
		if err != nil {
			return err
		}

		// grab images from each filtered component
		for _, component := range filteredComponents {
			pkgImgMap[pkg.Name] = append(pkgImgMap[pkg.Name], component.Images...)
		}

	}

	pkgImgsOut, err := goyaml.Marshal(pkgImgMap)
	if err != nil {
		return err
	}
	fmt.Println(string(pkgImgsOut))
	return nil
}

// listVariables prints the variables and overrides for each package in the bundle
func (b *Bundle) listVariables() error {
	message.HorizontalRule()
	message.Title("Overrides and Variables:", "configurable helm overrides and Zarf variables by package")

	for _, pkg := range b.bundle.Packages {

		zarfPkg, err := b.getMetadata(pkg)
		if err != nil {
			return err
		}

		variables := make([]interface{}, 0)

		// add each zarf var to variables for better formatting in output
		for _, zarfVar := range zarfPkg.Variables {
			variables = append(variables, zarfVar)
		}

		if len(pkg.Overrides) > 0 {
			variables = append(variables, pkg.Overrides)
		}

		varMap := map[string]map[string]interface{}{pkg.Name: {"variables": variables}}
		if err := zarfUtils.ColorPrintYAML(varMap, nil, false); err != nil {
			message.Warn("error printing variables")
		}
	}

	return nil
}

func (b *Bundle) getMetadata(pkg types.Package) (v1alpha1.ZarfPackage, error) {
	// if we are inspecting a built bundle, get the metadata from the bundle
	if !b.cfg.InspectOpts.IsYAMLFile {
		pkgTmp, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
		if err != nil {
			return v1alpha1.ZarfPackage{}, err
		}
		defer os.RemoveAll(pkgTmp)

		opts := zarfTypes.ZarfPackageOptions{
			PackageSource:      pkgTmp,
			OptionalComponents: strings.Join(pkg.OptionalComponents, ","),
			PublicKeyPath:      "",
		}

		sha := strings.Split(pkg.Ref, "@sha256:")[1] // using appended SHA from create!
		source, err := sources.NewFromLocation(*b.cfg, pkg, opts, sha, nil)
		if err != nil {
			return v1alpha1.ZarfPackage{}, err
		}
		zarfPkg, _, err := source.LoadPackageMetadata(context.TODO(), false, true)
		if err != nil {
			return v1alpha1.ZarfPackage{}, err
		}

		return zarfPkg, nil
	}

	// otherwise we are inspecting a yaml file, get the metadata from the packages directly
	sourceDir := strings.TrimSuffix(b.cfg.InspectOpts.Source, config.BundleYAML)

	source, err := getPkgSource(pkg, config.GetArch(b.bundle.Metadata.Architecture), sourceDir)
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}

	remoteOpts := packager.RemoteOptions{
		PlainHTTP:             config.CommonOptions.Insecure,
		InsecureSkipTLSVerify: config.CommonOptions.Insecure,
	}

	loadOpts := packager.LoadOptions{
		Filter:                  filters.Empty(),
		SkipSignatureValidation: false,
		Architecture:            config.GetArch(b.bundle.Metadata.Architecture),
		PublicKeyPath:           b.cfg.DeployOpts.PublicKeyPath,
		CachePath:               config.CommonOptions.CachePath,
		RemoteOptions:           remoteOpts,
	}

	pkgLayout, err := packager.LoadPackage(context.TODO(), source, loadOpts)
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}

	defer pkgLayout.Cleanup()

	return pkgLayout.Pkg, nil
}
