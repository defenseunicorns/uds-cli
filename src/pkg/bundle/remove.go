// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"fmt"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/pkg/state"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/message"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
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
			return fmt.Errorf("invalid zarf packages specified by --packages")
		}
	} else {
		packagesToRemove = b.bundle.Packages
	}

	// get bundle state
	kc, err := cluster.NewCluster()
	if err != nil {
		return err
	}
	sc, err := state.NewClient(kc.Clientset)
	if err != nil {
		return err
	}

	err = sc.InitBundleState(b.bundle.Metadata.Name)
	if err != nil {
		return err
	}

	err = sc.UpdateBundleState(b.bundle.Metadata.Name, state.Removing)
	if err != nil {
		return err
	}

	// remove packages
	removeErr := removePackages(sc, packagesToRemove, b)
	if removeErr != nil {
		return removeErr
	}

	// remove bundle state secret
	warns, err := sc.RemoveBundleState(b.bundle.Metadata.Name)
	if err != nil {
		return err
	}

	// print warnings
	for _, warn := range warns {
		message.Warn(warn)
	}

	return nil
}

func removePackages(sc *state.Client, packagesToRemove []types.Package, b *Bundle) error {
	// Get deployed packages from Zarf state
	deployedPackageNames := GetDeployedPackageNames()

	// also check bundle state
	bundleState, err := sc.GetBundleState(b.bundle.Metadata.Name)
	if err != nil {
		return err
	}
	for _, pkg := range bundleState.PkgStatuses {
		deployedPackageNames = append(deployedPackageNames, pkg.Name)
	}

	for i := len(packagesToRemove) - 1; i >= 0; i-- {
		pkg := packagesToRemove[i]

		if slices.Contains(deployedPackageNames, pkg.Name) {
			err := sc.UpdateBundlePkgState(b.bundle.Metadata.Name, pkg.Name, state.Removing)
			if err != nil {
				return err
			}

			opts := zarfTypes.ZarfPackageOptions{
				PackageSource: b.cfg.RemoveOpts.Source,
			}
			pkgCfg := zarfTypes.PackagerConfig{
				PkgOpts: opts,
			}
			pkgTmp, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
			if err != nil {
				return err
			}

			sha := strings.Split(pkg.Ref, "sha256:")[1]
			source, err := sources.New(*b.cfg, pkg, opts, sha, nil)
			if err != nil {
				return err
			}

			pkgClient, err := packager.New(&pkgCfg, packager.WithSource(source), packager.WithTemp(pkgTmp))
			if err != nil {
				return err
			}
			defer pkgClient.ClearTempPaths()

			if err := pkgClient.Remove(context.TODO()); err != nil {
				return err
			}

			err = sc.UpdateBundlePkgState(b.bundle.Metadata.Name, pkg.Name, state.Removed)

		} else {
			message.Warnf("Skipping removal of %s. Package not deployed", pkg.Name)
		}
	}

	return nil
}
