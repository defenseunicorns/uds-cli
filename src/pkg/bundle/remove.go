// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"fmt"
	"strings"

	"github.com/defenseunicorns/zarf/src/pkg/packager"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	"golang.org/x/exp/slices"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/types"
)

// Remove removes packages deployed from a bundle
func (b *Bundler) Remove() error {
	ctx := context.TODO()
	// create a new provider
	provider, err := NewBundleProvider(ctx, b.cfg.RemoveOpts.Source, b.tmp)
	if err != nil {
		return err
	}

	// pull the bundle's metadata + sig
	loaded, err := provider.LoadBundleMetadata()
	if err != nil {
		return err
	}

	// read the bundle's metadata into memory
	if err := utils.ReadYaml(loaded[config.BundleYAML], &b.bundle); err != nil {
		return err
	}

	// Check if --packages flag is set and zarf packages have been specified
	var packagesToRemove []types.BundleZarfPackage

	if len(b.cfg.RemoveOpts.Packages) != 0 {
		userSpecifiedPackages := strings.Split(b.cfg.RemoveOpts.Packages[0], ",")
		for _, pkg := range b.bundle.ZarfPackages {
			if slices.Contains(userSpecifiedPackages, pkg.Name) {
				packagesToRemove = append(packagesToRemove, pkg)
			}
		}

		// Check if invalid packages were specified
		if len(userSpecifiedPackages) != len(packagesToRemove) {
			return fmt.Errorf("invalid zarf packages specified by --packages")
		}
		return removePackages(packagesToRemove, b)
	} 
	return removePackages(b.bundle.ZarfPackages, b)
}

func removePackages(packagesToRemove []types.BundleZarfPackage, b *Bundler) error {
	for i := len(packagesToRemove) - 1; i >= 0; i-- {
		pkg := packagesToRemove[i]

		opts := zarfTypes.ZarfPackageOptions{
			PackageSource: b.cfg.RemoveOpts.Source,
		}
		pkgCfg := zarfTypes.PackagerConfig{
			PkgOpts: opts,
		}
		pkgTmp, err := utils.MakeTempDir(config.CommonOptions.TempDirectory)
		if err != nil {
			return err
		}

		sha := strings.Split(pkg.Ref, "sha256:")[1]
		source, err := sources.New(b.cfg.RemoveOpts.Source, pkg.Name, opts, sha)
		if err != nil {
			return err
		}

		pkgClient := packager.NewOrDie(&pkgCfg, packager.WithSource(source), packager.WithTemp(pkgTmp))
		if err != nil {
			return err
		}
		defer pkgClient.ClearTempPaths()

		if err := pkgClient.Remove(); err != nil {
			return err
		}
	}

	return nil
}
