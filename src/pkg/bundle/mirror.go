// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/types"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
	"golang.org/x/exp/slices"
)

// Deploy deploys a bundle
func (b *Bundle) Mirror(ctx context.Context) error {
	packagesToDeploy := b.bundle.Packages

	// Check if --packages flag is set and zarf packages have been specified
	if len(b.cfg.DeployOpts.Packages) != 0 {
		userSpecifiedPackages := strings.Split(strings.ReplaceAll(b.cfg.DeployOpts.Packages[0], " ", ""), ",")
		var selectedPackages []types.Package
		for _, pkg := range b.bundle.Packages {
			if slices.Contains(userSpecifiedPackages, pkg.Name) {
				selectedPackages = append(selectedPackages, pkg)
			}
		}

		packagesToDeploy = selectedPackages

		// Check if invalid packages were specified
		if len(userSpecifiedPackages) != len(packagesToDeploy) {
			return errors.New("invalid zarf packages specified by --packages")
		}
	}

	// if resume, filter for packages not yet deployed
	if b.cfg.DeployOpts.Resume {
		deployedPackageNames := GetDeployedPackageNames()
		var notDeployed []types.Package

		for _, pkg := range packagesToDeploy {
			if !slices.Contains(deployedPackageNames, pkg.Name) {
				notDeployed = append(notDeployed, pkg)
			}
			packagesToDeploy = notDeployed
		}
	}

	return mirrorPackages(ctx, packagesToDeploy, b)
}

func mirrorPackages(ctx context.Context, packagesToDeploy []types.Package, b *Bundle) error {
	// map of Zarf pkgs and their vars
	bundleExportedVars := make(map[string]map[string]string)

	// setup each package client and deploy
	for i, pkg := range packagesToDeploy {
		// for dev mode update package ref for remote bundles, refs for local bundles updated on create
		if config.Dev && !strings.Contains(b.cfg.DeployOpts.Source, "tar.zst") {
			pkg, err := b.setPackageRef(pkg)
			if err != nil {
				return err
			}
			b.bundle.Packages[i] = pkg
		}
		sha := strings.Split(pkg.Ref, "@sha256:")[1] // using appended SHA from create!
		pkgTmp, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
		if err != nil {
			return err
		}
		defer os.RemoveAll(pkgTmp)

		publicKeyPath := filepath.Join(b.tmp, config.PublicKeyFile)
		if pkg.PublicKey != "" {
			if err := os.WriteFile(publicKeyPath, []byte(pkg.PublicKey), helpers.ReadWriteUser); err != nil {
				return err
			}
			defer os.Remove(publicKeyPath)
		} else {
			publicKeyPath = ""
		}

		// pkgVars, variableData := b.loadVariables(pkg, bundleExportedVars)

		// valuesOverrides, nsOverrides, err := b.loadChartOverrides(pkg, variableData)
		// if err != nil {
		// 	return err
		// }

		opts := zarfTypes.ZarfPackageOptions{
			PackageSource:      pkgTmp,
			OptionalComponents: strings.Join(pkg.OptionalComponents, ","),
			PublicKeyPath:      publicKeyPath,
			// SetVariables:       pkgVars,
			Retries: b.cfg.DeployOpts.Retries,
		}

		// zarfDeployOpts := zarfTypes.ZarfDeployOptions{
		// 	ValuesOverridesMap: valuesOverrides,
		// 	Timeout:            config.HelmTimeout,
		// }

		// Automatically confirm the package deployment
		zarfConfig.CommonOptions.Confirm = true

		source, err := sources.NewFromLocation(*b.cfg, pkg, opts, sha, nil)
		if err != nil {
			return err
		}

		// pkgCfg := zarfTypes.PackagerConfig{
		// 	PkgOpts:    opts,
		// 	DeployOpts: zarfDeployOpts,
		// }

		// // handle zarf init configs that aren't Zarf variables
		// zarfPkg, _, err := source.LoadPackageMetadata(context.TODO(), layout.New(pkgTmp), false, false)
		// if err != nil {
		// 	return err
		// }

		// zarfInitOpts := handleZarfInitOpts(pkgVars, zarfPkg.Kind)
		// pkgCfg.InitOpts = zarfInitOpts

		// pkgCfg.DeployOpts.RegistryURL = zarfInitOpts.RegistryInfo.Address

		// for k, v := range pkgVars {
		// 	switch k {
		// 	// registry info
		// 	case config.RegistryURL:
		// 		zarfInitOpts.RegistryInfo.Address = v
		// 	case config.RegistryPushUsername:
		// 		zarfInitOpts.RegistryInfo.PushUsername = v
		// 	case config.RegistryPushPassword:
		// 		zarfInitOpts.RegistryInfo.PushPassword = v
		// 	case config.RegistryPullUsername:
		// 		zarfInitOpts.RegistryInfo.PullUsername = v
		// 	case config.RegistryPullPassword:
		// 		zarfInitOpts.RegistryInfo.PullPassword = v
		// 	case config.RegistrySecretName:
		// 		zarfInitOpts.RegistryInfo.Secret = v
		// 	case config.RegistryNodeport:
		// 		np, err := strconv.Atoi(v)
		// 		if err != nil {
		// 			message.Warnf("failed to parse nodeport %s: %v", v, err)
		// 			return zarfTypes.ZarfInitOptions{}
		// 		}
		// 		zarfInitOpts.RegistryInfo.NodePort = np
		// 	// git server info
		// 	case config.GitURL:
		// 		zarfInitOpts.GitServer.Address = v
		// 	case config.GitPushUsername:
		// 		zarfInitOpts.GitServer.PushUsername = v
		// 	case config.GitPushPassword:
		// 		zarfInitOpts.GitServer.PushPassword = v
		// 	case config.GitPullUsername:
		// 		zarfInitOpts.GitServer.PullUsername = v
		// 	case config.GitPullPassword:
		// 		zarfInitOpts.GitServer.PullPassword = v
		// 	// artifact server info
		// 	case config.ArtifactURL:
		// 		zarfInitOpts.ArtifactServer.Address = v
		// 	case config.ArtifactPushUsername:
		// 		zarfInitOpts.ArtifactServer.PushUsername = v
		// 	case config.ArtifactPushToken:
		// 		zarfInitOpts.ArtifactServer.PushToken = v
		// 	// storage class
		// 	case config.StorageClass:
		// 		zarfInitOpts.StorageClass = v
		// 	}
		// }

		pkgCfg := zarfTypes.PackagerConfig{
			PkgOpts: opts,
		}

		pkgClient, err := packager.New(&pkgCfg, packager.WithSource(source), packager.WithTemp(opts.PackageSource))
		if err != nil {
			return err
		}

		if err = pkgClient.Mirror(ctx); err != nil {
			return err
		}

		// save exported vars
		pkgExportedVars := make(map[string]string)
		variableConfig := pkgClient.GetVariableConfig()
		for _, exp := range pkg.Exports {
			// ensure if variable exists in package
			setVariable, ok := variableConfig.GetSetVariable(exp.Name)
			if !ok {
				return fmt.Errorf("cannot export variable %s because it does not exist in package %s", exp.Name, pkg.Name)
			}
			pkgExportedVars[strings.ToUpper(exp.Name)] = setVariable.Value
		}
		bundleExportedVars[pkg.Name] = pkgExportedVars
	}
	return nil
}
