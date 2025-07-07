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
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	goyaml "github.com/goccy/go-yaml"
	"github.com/pterm/pterm"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
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

		// zarfPkg, err := loadPackage(*b, pkg)
		// if err != nil {
		// 	return err
		// }

		// create filter for optional components
		inspectFilter := filters.Combine(
			filters.ForDeploy(strings.Join(pkg.OptionalComponents, ","), false),
		)

		// TODO: determine better way to delineate local vs remote packages
		source, err := getPkgSource(pkg, config.GetArch(b.bundle.Metadata.Architecture), b.cfg.InspectOpts.Source)
		if err != nil {
			return err
		}

		remoteOpts := packager.RemoteOptions{
			PlainHTTP:             config.CommonOptions.Insecure,
			InsecureSkipTLSVerify: config.CommonOptions.Insecure,
		}

		loadOpts := packager.LoadOptions{
			Filter:                  inspectFilter,
			SkipSignatureValidation: false,
			Architecture:            config.GetArch(),
			PublicKeyPath:           b.cfg.DeployOpts.PublicKeyPath,
			CachePath:               config.CommonOptions.CachePath,
			RemoteOptions:           remoteOpts,
		}

		pkgLayout, err := packager.LoadPackage(context.TODO(), source, loadOpts)
		if err != nil {
			return err
		}

		for _, component := range pkgLayout.Pkg.Components {
			pkgImgMap[pkg.Name] = append(pkgImgMap[pkg.Name], component.Images...)
		}

		// filteredComponents, err := inspectFilter.Apply(zarfPkg)
		// if err != nil {
		// 	return err
		// }

		// grab images from each filtered component
		// for _, component := range filteredComponents {
		// 	pkgImgMap[pkg.Name] = append(pkgImgMap[pkg.Name], component.Images...)
		// }

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

		source, err := getPkgSource(pkg, config.GetArch(b.bundle.Metadata.Architecture), b.cfg.CreateOpts.SourceDirectory)
		if err != nil {
			return err
		}

		remoteOpts := packager.RemoteOptions{
			PlainHTTP:             config.CommonOptions.Insecure,
			InsecureSkipTLSVerify: config.CommonOptions.Insecure,
		}

		loadOpts := packager.LoadOptions{
			Filter:                  filters.Empty(),
			SkipSignatureValidation: false,
			Architecture:            config.GetArch(),
			PublicKeyPath:           b.cfg.DeployOpts.PublicKeyPath,
			CachePath:               config.CommonOptions.CachePath,
			RemoteOptions:           remoteOpts,
		}

		pkgLayout, err := packager.LoadPackage(context.TODO(), source, loadOpts)
		if err != nil {
			return err
		}

		variables := make([]interface{}, 0)

		// add each zarf var to variables for better formatting in output
		for _, zarfVar := range pkgLayout.Pkg.Variables {
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

// func loadPackage(b Bundle, pkg types.Package) (v1alpha1.ZarfPackage, error) {
// 	var source sources.PackageSource
// 	source, err := b.getSource(pkg)
// 	if err != nil {
// 		return v1alpha1.ZarfPackage{}, err
// 	}

// 	tmpDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
// 	if err != nil {
// 		return v1alpha1.ZarfPackage{}, err
// 	}
// 	pkgPaths := layout.New(tmpDir)
// 	defer os.RemoveAll(tmpDir)

// 	zarfPkg, _, err := source.LoadPackageMetadata(context.TODO(), pkgPaths, false, true)
// 	if err != nil {
// 		return v1alpha1.ZarfPackage{}, err
// 	}

// 	return zarfPkg, nil
// }

// // getSource returns a package source based on if inspecting bundle yaml or bundle artifact
// func (b *Bundle) getSource(pkg types.Package) (sources.PackageSource, error) {
// 	var source sources.PackageSource

// 	// If the inspect target is not a yaml file
// 	if !b.cfg.InspectOpts.IsYAMLFile {
// 		sha := strings.Split(pkg.Ref, "@sha256:")[1] // using appended SHA from create!
// 		fromTarball, err := sources.NewFromLocation(*b.cfg, pkg, zarfTypes.ZarfPackageOptions{}, sha, nil)
// 		if err != nil {
// 			return nil, err
// 		}
// 		source = fromTarball
// 	} else {
// 		if pkg.Repository != "" {
// 			// handle remote packages
// 			url := fmt.Sprintf("oci://%s:%s", pkg.Repository, pkg.Ref)
// 			platform := ocispec.Platform{
// 				Architecture: config.GetArch(),
// 				OS:           oci.MultiOS,
// 			}
// 			remote, err := zoci.NewRemote(context.TODO(), url, platform)
// 			if err != nil {
// 				return nil, err
// 			}

// 			source = &zarfSources.OCISource{
// 				ZarfPackageOptions: &zarfTypes.ZarfPackageOptions{},
// 				Remote:             remote,
// 			}
// 		} else if pkg.Path != "" {
// 			// handle local packages
// 			err := os.Chdir(filepath.Dir(b.cfg.InspectOpts.Source)) // change to the bundle's directory
// 			if err != nil {
// 				return nil, err
// 			}

// 			bundleArch := config.GetArch(b.bundle.Metadata.Architecture)
// 			tarballName := fmt.Sprintf("zarf-package-%s-%s-%s.tar.zst", pkg.Name, bundleArch, pkg.Ref)
// 			source = &zarfSources.TarballSource{
// 				ZarfPackageOptions: &zarfTypes.ZarfPackageOptions{
// 					PackageSource: filepath.Join(pkg.Path, tarballName),
// 				},
// 			}
// 		} else {
// 			return nil, fmt.Errorf("package %s is missing a repository or path", pkg.Name)
// 		}
// 	}

// 	return source, nil
// }
