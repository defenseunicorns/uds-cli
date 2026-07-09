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
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
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

		// Pre-warm per-package metadata in a single pass through the bundle.
		// On large bundles this is the dominant cost of `uds inspect`.
		// We only do this for tarball sources — remote (OCI) providers already
		// fetch metadata blobs independently.
		needsPkgMeta := b.cfg.InspectOpts.ListVariables || b.cfg.InspectOpts.ListImages || !config.CommonOptions.SkipSignatureValidation
		if needsPkgMeta {
			if tp, ok := provider.(*tarballBundleProvider); ok {
				cache, perr := tp.prefetchPackageMetadata(context.TODO(), b.bundle.Packages, b.tmp)
				if perr != nil {
					// Fall back to the per-package slow path on any error so
					// inspect still works on bundles the prefetcher can't handle.
					message.Warnf("package metadata prefetch failed, falling back to per-package extraction: %v", perr)
				} else {
					b.pkgMetaCache = cache
				}
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

	// If the user is not skipping validation amd did not choose a mode that already
	// loaded package metadata (like --list-variables/--list-images), verify packages now.
	if !config.CommonOptions.SkipSignatureValidation {
		for _, pkg := range b.bundle.Packages {
			if _, err := b.getMetadata(pkg); err != nil {
				return err
			}
		}
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
			pkgImgMap[pkg.Name] = append(pkgImgMap[pkg.Name], component.GetImages()...)
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
	// Fast path: metadata was pre-fetched for the whole bundle in a single
	// pass. Verify and load it the same keyless-capable way as the non-cached
	// path below, just against the directory the prefetcher already wrote so we
	// don't re-extract from the (multi-GB) bundle tarball per package.
	if cached, ok := b.pkgMetaCache[pkg.Name]; ok {
		keyTmp, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
		if err != nil {
			return v1alpha1.ZarfPackage{}, err
		}
		defer os.RemoveAll(keyTmp)

		verifyOpts, err := utils.BuildVerifyBlobOptions(pkg, keyTmp)
		if err != nil {
			return v1alpha1.ZarfPackage{}, fmt.Errorf("package %q: %w", pkg.Name, err)
		}

		pkgLayout, err := utils.LoadPackageFromDir(context.TODO(), cached.dirPath, layout.PackageLayoutOptions{
			IsPartial:            true,
			VerificationStrategy: utils.GetPackageVerificationStrategy(config.CommonOptions.SkipSignatureValidation),
			VerifyBlobOptions:    verifyOpts,
		})
		if err != nil {
			return v1alpha1.ZarfPackage{}, fmt.Errorf("package %q: %w", pkg.Name, err)
		}
		return pkgLayout.Pkg, nil
	}

	// if we are inspecting a built bundle, get the metadata from the bundle
	if !b.cfg.InspectOpts.IsYAMLFile {
		pkgTmp, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
		if err != nil {
			return v1alpha1.ZarfPackage{}, err
		}
		defer os.RemoveAll(pkgTmp)

		// BuildVerifyBlobOptions may write a key file to the given dir; use a separate
		// temp dir so pkgTmp contains only package files and passes LoadFromDir integrity checks.
		keyTmp, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
		if err != nil {
			return v1alpha1.ZarfPackage{}, err
		}
		defer os.RemoveAll(keyTmp)

		verifyOpts, err := utils.BuildVerifyBlobOptions(pkg, keyTmp)
		if err != nil {
			return v1alpha1.ZarfPackage{}, fmt.Errorf("package %q: %w", pkg.Name, err)
		}

		sha := strings.Split(pkg.Ref, "@sha256:")[1] // using appended SHA from create!
		source, err := sources.NewFromLocation(*b.cfg, pkg, pkgTmp, verifyOpts, config.CommonOptions.SkipSignatureValidation, sha, nil)
		if err != nil {
			return v1alpha1.ZarfPackage{}, err
		}

		if _, _, err := source.LoadPackageMetadata(context.TODO(), false, true); err != nil {
			return v1alpha1.ZarfPackage{}, err
		}

		pkgLayout, err := utils.LoadPackageFromDir(context.TODO(), pkgTmp, layout.PackageLayoutOptions{
			IsPartial:            true,
			VerificationStrategy: utils.GetPackageVerificationStrategy(config.CommonOptions.SkipSignatureValidation),
			VerifyBlobOptions:    verifyOpts,
		})
		if err != nil {
			return v1alpha1.ZarfPackage{}, fmt.Errorf("package %q: %w", pkg.Name, err)
		}

		return pkgLayout.Pkg, nil
	}

	// otherwise we are inspecting a yaml file, get the metadata from the packages directly
	sourceDir := strings.TrimSuffix(b.cfg.InspectOpts.Source, config.BundleYAML)

	source, err := utils.GetPkgSource(pkg, config.GetArch(b.bundle.Metadata.Architecture), sourceDir)
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}

	yamlTmp, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}
	defer os.RemoveAll(yamlTmp)

	verifyOpts, err := utils.BuildVerifyBlobOptions(pkg, yamlTmp)
	if err != nil {
		return v1alpha1.ZarfPackage{}, fmt.Errorf("package %q: %w", pkg.Name, err)
	}

	remoteOpts := zarfTypes.RemoteOptions{
		PlainHTTP:             config.CommonOptions.Insecure,
		InsecureSkipTLSVerify: config.CommonOptions.Insecure,
	}

	loadOpts := packager.LoadOptions{
		Filter:               filters.Empty(),
		VerificationStrategy: utils.GetPackageVerificationStrategy(config.CommonOptions.SkipSignatureValidation),
		Architecture:         config.GetArch(b.bundle.Metadata.Architecture),
		VerifyBlobOptions:    verifyOpts,
		CachePath:            config.CommonOptions.CachePath,
		RemoteOptions:        remoteOpts,
		OCIConcurrency:       config.CommonOptions.OCIConcurrency,
	}

	pkgLayout, err := utils.LoadPackage(context.TODO(), source, loadOpts)
	if err != nil {
		return v1alpha1.ZarfPackage{}, fmt.Errorf("package %q: %w", pkg.Name, err)
	}

	err = pkgLayout.Cleanup()
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}

	return pkgLayout.Pkg, nil
}
