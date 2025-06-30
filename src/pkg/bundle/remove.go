// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"golang.org/x/exp/slices"
)

// Remove removes packages deployed from a bundle
func (b *Bundle) Remove() error {
	// Check that provided oci source path is valid, and update it if it's missing the full path
	source, err := CheckOCISourcePath(b.cfg.RemoveOpts.Source)
	if err != nil {
		return err
	}
	b.cfg.RemoveOpts.Source = source

	// validate CLI config's arch against cluster
	err = ValidateArch(config.GetArch())
	if err != nil {
		return err
	}

	// create a new provider
	provider, err := NewBundleProvider(b.cfg.RemoveOpts.Source, b.tmp)
	if err != nil {
		return err
	}

	// pull the bundle's metadata + sig
	filepaths, err := provider.LoadBundleMetadata()
	if err != nil {
		return err
	}

	// read the bundle's metadata into memory
	if err := utils.ReadYAMLStrict(filepaths[config.BundleYAML], &b.bundle); err != nil {
		return err
	}

	// Check if --packages flag is set and zarf packages have been specified
	var packagesToRemove []types.Package

	if len(b.cfg.RemoveOpts.Packages) != 0 {
		userSpecifiedPackages := strings.Split(strings.ReplaceAll(b.cfg.RemoveOpts.Packages[0], " ", ""), ",")
		for _, pkg := range b.bundle.Packages {
			if slices.Contains(userSpecifiedPackages, pkg.Name) {
				packagesToRemove = append(packagesToRemove, pkg)
			}
		}

		// Check if invalid packages were specified
		if len(userSpecifiedPackages) != len(packagesToRemove) {
			return errors.New("invalid zarf packages specified by --packages")
		}
		return removePackages(packagesToRemove, b)
	}
	return removePackages(b.bundle.Packages, b)
}

func removePackages(packagesToRemove []types.Package, b *Bundle) error {
	ctx := context.TODO()
	// Get deployed packages
	deployedPackageNames := GetDeployedPackageNames()

	for i := len(packagesToRemove) - 1; i >= 0; i-- {
		pkg := packagesToRemove[i]

		if slices.Contains(deployedPackageNames, pkg.Name) {
			// opts := zarfTypes.ZarfPackageOptions{
			// 	PackageSource: b.cfg.RemoveOpts.Source,
			// }
			// pkgCfg := zarfTypes.PackagerConfig{
			// 	PkgOpts: opts,
			// }
			// pkgTmp, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
			// if err != nil {
			// 	return err
			// }

			// sha := strings.Split(pkg.Ref, "sha256:")[1]
			// source, err := sources.NewFromLocation(*b.cfg, pkg, opts, sha, nil)
			// if err != nil {
			// 	return err
			// }

			// pkgClient, err := packager.New(&pkgCfg, packager.WithSource(source), packager.WithTemp(pkgTmp))
			// if err != nil {
			// 	return err
			// }
			// defer pkgClient.ClearTempPaths()

			// if err := pkgClient.Remove(context.TODO()); err != nil {
			// 	return err
			// }

			filter := filters.Combine(
				filters.ByLocalOS(runtime.GOOS),
				// filters.BySelectState(pkgConfig.PkgOpts.OptionalComponents),
			)

			c, _ := cluster.New(ctx) //nolint:errcheck
			loadOpts := packager.LoadOptions{
				SkipSignatureValidation: false,
				Architecture:            config.GetArch(),
				Filter:                  filter,
				// PublicKeyPath:           pkgConfig.PkgOpts.PublicKeyPath,
				OCIConcurrency: config.CommonOptions.OCIConcurrency,
				// RemoteOptions:           defaultRemoteOptions(),
				// CachePath:               cachePath,
			}

			// TODO: validate that package name is the same as the pkg.metadata.name
			pkg, err := packager.GetPackageFromSourceOrCluster(ctx, c, pkg.Name, loadOpts)
			if err != nil {
				return fmt.Errorf("unable to load the package: %w", err)
			}
			removeOpt := packager.RemoveOptions{
				Cluster: c,
				Timeout: config.HelmTimeout,
			}
			err = packager.Remove(ctx, pkg, removeOpt)
			if err != nil {
				return err
			}

		} else {
			message.Warnf("Skipping removal of %s. Package not deployed", pkg.Name)
		}
	}

	return nil
}
