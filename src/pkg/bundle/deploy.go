// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/pterm/pterm"
	"golang.org/x/exp/maps"

	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/packager"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/types"
)

// Deploy deploys a bundle
//
// : create a new provider
// : pull the bundle's metadata + sig
// : read the metadata into memory
// : validate the sig (if present)
// : loop through each package
// : : load the package into a fresh temp dir
// : : validate the sig (if present)
// : : deploy the package
func (b *Bundler) Deploy() error {
	ctx := context.TODO()

	pterm.Println()
	metadataSpinner := message.NewProgressSpinner("Loading bundle metadata")

	defer metadataSpinner.Stop()

	// create a new provider
	provider, err := NewBundleProvider(ctx, b.cfg.DeployOpts.Source, b.tmp)
	if err != nil {
		return err
	}

	// pull the bundle's metadata + sig
	loaded, err := provider.LoadBundleMetadata()
	if err != nil {
		return err
	}

	// validate the sig (if present)
	if err := ValidateBundleSignature(loaded[config.BundleYAML], loaded[config.BundleYAMLSignature], b.cfg.DeployOpts.PublicKeyPath); err != nil {
		return err
	}

	// read the bundle's metadata into memory
	if err := utils.ReadYaml(loaded[config.BundleYAML], &b.bundle); err != nil {
		return err
	}

	metadataSpinner.Successf("Loaded bundle metadata")

	// confirm deploy
	if ok := b.confirmBundleDeploy(); !ok {
		return fmt.Errorf("bundle deployment cancelled")
	}

	// map of Zarf pkgs and their vars
	bundleExportedVars := make(map[string]map[string]string)

	// deploy each package
	for _, pkg := range b.bundle.ZarfPackages {
		sha := strings.Split(pkg.Ref, "@sha256:")[1] // using appended SHA from create!
		pkgTmp, err := utils.MakeTempDir(config.CommonOptions.TempDirectory)
		if err != nil {
			return err
		}
		defer os.RemoveAll(pkgTmp)

		publicKeyPath := filepath.Join(b.tmp, config.PublicKeyFile)
		if pkg.PublicKey != "" {
			if err := utils.WriteFile(publicKeyPath, []byte(pkg.PublicKey)); err != nil {
				return err
			}
			defer os.Remove(publicKeyPath)
		} else {
			publicKeyPath = ""
		}

		pkgVars := b.loadVariables(pkg, bundleExportedVars)

		opts := zarfTypes.ZarfPackageOptions{
			PackageSource:      pkgTmp,
			OptionalComponents: strings.Join(pkg.OptionalComponents, ","),
			PublicKeyPath:      publicKeyPath,
			SetVariables:       pkgVars,
		}
		pkgCfg := zarfTypes.PackagerConfig{
			PkgOpts:  opts,
			InitOpts: config.DefaultZarfInitOptions,
		}

		// grab Zarf version to make Zarf library checks happy
		if buildInfo, ok := debug.ReadBuildInfo(); ok {
			for _, dep := range buildInfo.Deps {
				if dep.Path == "github.com/defenseunicorns/zarf" {
					zarfConfig.CLIVersion = strings.Split(dep.Version, "v")[1]
				}
			}
		}

		// Automatically confirm the package deployment
		zarfConfig.CommonOptions.Confirm = true

		source, err := sources.New(b.cfg.DeployOpts.Source, pkg.Name, opts, sha)
		if err != nil {
			return err
		}

		pkgClient := packager.NewOrDie(&pkgCfg, packager.WithSource(source), packager.WithTemp(opts.PackageSource))
		if err != nil {
			return err
		}
		if err := pkgClient.Deploy(); err != nil {
			return err
		}

		// save exported vars
		pkgExportedVars := make(map[string]string)
		for _, exp := range pkg.Exports {
			pkgExportedVars[strings.ToUpper(exp.Name)] = pkgCfg.SetVariableMap[exp.Name].Value
		}
		bundleExportedVars[pkg.Name] = pkgExportedVars
	}
	return nil
}

// loadVariables loads and sets precedence for config-level and imported variables
func (b *Bundler) loadVariables(pkg types.BundleZarfPackage, bundleExportedVars map[string]map[string]string) map[string]string {
	pkgVars := make(map[string]string)
	pkgConfigVars := make(map[string]string)
	for name, val := range b.cfg.DeployOpts.ZarfPackageVariables[pkg.Name].Set {
		pkgConfigVars[strings.ToUpper(name)] = val
	}
	pkgImportedVars := make(map[string]string)
	for _, imp := range pkg.Imports {
		pkgImportedVars[strings.ToUpper(imp.Name)] = bundleExportedVars[imp.Package][imp.Name]
	}

	// set var precedence
	maps.Copy(pkgVars, pkgImportedVars)
	maps.Copy(pkgVars, pkgConfigVars)
	return pkgVars
}

// confirmBundleDeploy prompts the user to confirm bundle creation
func (b *Bundler) confirmBundleDeploy() (confirm bool) {

	message.HeaderInfof("🎁 BUNDLE DEFINITION")
	utils.ColorPrintYAML(b.bundle, nil, false)

	message.HorizontalRule()

	// Display prompt if not auto-confirmed
	if config.CommonOptions.Confirm {
		return config.CommonOptions.Confirm
	}

	prompt := &survey.Confirm{
		Message: fmt.Sprintf("Deploy this bundle?"),
	}

	pterm.Println()

	if err := survey.AskOne(prompt, &confirm); err != nil || !confirm {
		return false
	}
	return true
}
