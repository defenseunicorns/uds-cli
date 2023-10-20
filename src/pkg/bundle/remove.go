// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"strings"

	"github.com/defenseunicorns/zarf/src/pkg/packager"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/types"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
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

	// remove in reverse order
	for i := len(b.bundle.ZarfPackages) - 1; i >= 0; i-- {
		pkg := b.bundle.ZarfPackages[i]
		if err != nil {
			return err
		}

		opts := types.ZarfPackageOptions{
			PackageSource: b.cfg.RemoveOpts.Source,
		}
		pkgCfg := types.PackagerConfig{
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
