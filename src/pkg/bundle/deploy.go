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
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"

	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/packager"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/types"
)

type ZarfOverrideMap map[string]map[string]map[string]interface{}

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

		valuesOverrides, err := b.loadChartOverrides(pkg)
		if err != nil {
			return err
		}

		zarfDeployOpts := zarfTypes.ZarfDeployOptions{
			ValuesOverridesMap: valuesOverrides,
		}

		pkgCfg := zarfTypes.PackagerConfig{
			PkgOpts:    opts,
			InitOpts:   config.DefaultZarfInitOptions,
			DeployOpts: zarfDeployOpts,
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
		Message: "Deploy this bundle?",
	}

	pterm.Println()

	if err := survey.AskOne(prompt, &confirm); err != nil || !confirm {
		return false
	}
	return true
}

// loadChartOverrides converts a helm path to a ValuesOverridesMap config for Zarf
func (b *Bundler) loadChartOverrides(pkg types.BundleZarfPackage) (ZarfOverrideMap, error) {

	// Create a nested map to hold the values
	overrideMap := make(map[string]map[string]*values.Options)

	// Loop through each path in Overrides.Values
	for _, override := range pkg.Overrides.Values {
		// Add the override to the map, or return an error if the path is invalid
		if err := addOverrideValue(overrideMap, override.Path, override.Value); err != nil {
			return nil, err
		}
	}

	// Loop through each path in Overrides.Variables
	for _, override := range pkg.Overrides.Variables {
		// Set the default value
		val := override.Default

		// If the variable is set, override the default value, why is this lowercase?
		name := strings.ToLower(override.Name)
		if setVal, ok := b.cfg.DeployOpts.ZarfPackageVariables[pkg.Name].Set[name]; ok {
			val = setVal
		}

		// Add the override to the map, or return an error if the path is invalid
		if err := addOverrideValue(overrideMap, override.Path, val); err != nil {
			return nil, err
		}
	}

	processed := make(ZarfOverrideMap)

	// Convert the options.Values map to the ZarfOverrideMap format
	for componentName, component := range overrideMap {
		// Create a map to hold all the charts in the component
		componentMap := make(map[string]map[string]interface{})

		// Loop through each chart in the component
		for chartName, chart := range component {
			// Merge the chart values with Helm
			data, err := chart.MergeValues(getter.Providers{})
			if err != nil {
				return nil, err
			}

			// Add the chart values to the component map
			componentMap[chartName] = data
		}

		// Add the component map to the processed map
		processed[componentName] = componentMap
	}

	return processed, nil
}

// addOverrideValue adds a value to a ZarfOverrideMap
func addOverrideValue(overrides map[string]map[string]*values.Options, path string, value interface{}) error {
	// Split the path string into <component-name>/<chart-name>/<value>
	// e.g. "nginx-ingress/nginx-ingress-controller/controller.service.type"
	// becomes ["nginx-ingress", "nginx-ingress-controller", "controller.service.type"]
	segments := strings.Split(path, "/")
	if len(segments) != 3 {
		// If the path is not in the correct format, return an error
		return fmt.Errorf("invalid helm path format: %s", path)
	}

	// Human-readable names for the segments of the path
	component, chart, valuePath := segments[0], segments[1], segments[2]

	// Create the component map if it doesn't exist
	if _, ok := overrides[component]; !ok {
		overrides[component] = make(map[string]*values.Options)
	}

	// Create the chart map if it doesn't exist
	if _, ok := overrides[component][chart]; !ok {
		overrides[component][chart] = &values.Options{}
	}

	// Add the value to the chart map
	helmVal := fmt.Sprintf("%s=%v", valuePath, value)
	overrides[component][chart].Values = append(overrides[component][chart].Values, helmVal)

	return nil
}
