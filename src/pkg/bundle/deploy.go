// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/types"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/packager"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	"github.com/pterm/pterm"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	// "oras.land/oras-go/v2/registry"
)

// ZarfOverrideMap is a map of Zarf packages -> components -> Helm charts -> values
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

	// Check that provided oci source path is valid, and update it if it's missing the full path
	b.cfg.DeployOpts.Source = CheckOCISourcePath(b.cfg.DeployOpts.Source)

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

	// Check if --resume is set
	resume := b.cfg.DeployOpts.Resume

	// Check if --packages flag is set and zarf packages have been specified
	var packagesToDeploy []types.BundleZarfPackage
	if len(b.cfg.DeployOpts.Packages) != 0 {
		userSpecifiedPackages := strings.Split(strings.ReplaceAll(b.cfg.DeployOpts.Packages[0], " ", ""), ",")

		for _, pkg := range b.bundle.ZarfPackages {
			if slices.Contains(userSpecifiedPackages, pkg.Name) {
				packagesToDeploy = append(packagesToDeploy, pkg)
			}
		}

		// Check if invalid packages were specified
		if len(userSpecifiedPackages) != len(packagesToDeploy) {
			return fmt.Errorf("invalid zarf packages specified by --packages")
		}
		return deployPackages(packagesToDeploy, resume, b)
	}

	return deployPackages(b.bundle.ZarfPackages, resume, b)
}

func deployPackages(packages []types.BundleZarfPackage, resume bool, b *Bundler) error {
	// map of Zarf pkgs and their vars
	bundleExportedVars := make(map[string]map[string]string)

	var packagesToDeploy []types.BundleZarfPackage

	if resume {
		deployedPackageNames := GetDeployedPackageNames()
		for _, pkg := range packages {
			if !slices.Contains(deployedPackageNames, pkg.Name) {
				packagesToDeploy = append(packagesToDeploy, pkg)
			}
		}
	} else {
		packagesToDeploy = packages
	}

	// deploy each package
	for _, pkg := range packagesToDeploy {
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
			Timeout:            config.HelmTimeout,
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
	// don't use pkg overrides here bc this function processes Zarf vars (even though they look the same in the uds-config)
	overrideVars := getPkgOverrideVars(pkg)
	for name, val := range b.cfg.DeployOpts.Variables[pkg.Name].Set {
		if !slices.Contains(overrideVars, name) {
			pkgConfigVars[strings.ToUpper(name)] = fmt.Sprint(val)
		}
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

func getPkgOverrideVars(pkg types.BundleZarfPackage) []string {
	var overrideVars []string
	for _, components := range pkg.Overrides {
		for _, chart := range components {
			for _, v := range chart.Variables {
				overrideVars = append(overrideVars, v.Name)
			}
		}
	}
	return overrideVars
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

	// Loop through each package component's charts and process overrides
	for componentName, component := range pkg.Overrides {
		for chartName, chart := range component {
			err := b.processOverrideValues(&overrideMap, &chart.Values, componentName, chartName)
			if err != nil {
				return nil, err
			}
			err = b.processOverrideVariables(&overrideMap, pkg.Name, &chart.Variables, componentName, chartName)
			if err != nil {
				return nil, err
			}
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

// processOverrideValues processes a bundles values overrides and adds them to the override map
func (b *Bundler) processOverrideValues(overrideMap *map[string]map[string]*values.Options, values *[]types.BundleChartValue, componentName string, chartName string) error {
	for _, v := range *values {
		// Add the override to the map, or return an error if the path is invalid
		if err := addOverrideValue(*overrideMap, componentName, chartName, v.Path, v.Value); err != nil {
			return err
		}
	}
	return nil
}

// processOverrideVariables processes a bundles variables overrides and adds them to the override map
func (b *Bundler) processOverrideVariables(overrideMap *map[string]map[string]*values.Options, pkgName string, variables *[]types.BundleChartVariable, componentName string, chartName string) error {
	for _, v := range *variables {
		var overrideVal interface{}
		// check for override in env vars
		if envVarOverride, exists := os.LookupEnv(strings.ToUpper(config.EnvVarPrefix + v.Name)); exists {
			if err := addOverrideValue(*overrideMap, componentName, chartName, v.Path, envVarOverride); err != nil {
				return err
			}
			continue
		}
		// check for override in config
		configFileOverride, existsInConfig := b.cfg.DeployOpts.Variables[pkgName].Set[v.Name]
		if v.Default == nil && !existsInConfig {
			// no default or config v, use values from underlying chart
			continue
		} else if existsInConfig {
			// if the config v is set, use it
			overrideVal = configFileOverride
		} else {
			// use default v if no config v is set
			overrideVal = v.Default
		}

		// Add the override to the map, or return an error if the path is invalid
		if err := addOverrideValue(*overrideMap, componentName, chartName, v.Path, overrideVal); err != nil {
			return err
		}

	}
	return nil
}

// addOverrideValue adds a value to a ZarfOverrideMap
func addOverrideValue(overrides map[string]map[string]*values.Options, component string, chart string, valuePath string, value interface{}) error {
	// Create the component map if it doesn't exist
	if _, ok := overrides[component]; !ok {
		overrides[component] = make(map[string]*values.Options)
	}

	// Create the chart map if it doesn't exist
	if _, ok := overrides[component][chart]; !ok {
		overrides[component][chart] = &values.Options{}
	}

	// Add the value to the chart map
	switch v := value.(type) {
	case []interface{}:
		// handle list of objects by parsing them as json and appending to Options.JSONValues
		jsonStrs := make([]string, len(v))
		// concat json strings representing items in the list
		for i, val := range v {
			j, err := json.Marshal(val)
			if err != nil {
				return err
			}
			jsonStrs[i] = fmt.Sprintf("%s", j)
		}
		// use JSONValues because we can easily marshal the YAML to JSON and Helm understands it
		jsonVals := fmt.Sprintf("%s=[%s]", valuePath, strings.Join(jsonStrs, ","))
		overrides[component][chart].JSONValues = append(overrides[component][chart].JSONValues, jsonVals)
	case map[string]interface{}:
		// handle objects by parsing them as json and appending to Options.JSONValues
		j, err := json.Marshal(v)
		if err != nil {
			return err
		}
		// use JSONValues because we can easily marshal the YAML to JSON and Helm understands it
		val := fmt.Sprintf("%s=%s", valuePath, j)
		overrides[component][chart].JSONValues = append(overrides[component][chart].JSONValues, val)
	default:
		// handle default case of simple values like strings and numbers
		helmVal := fmt.Sprintf("%s=%v", valuePath, value)
		overrides[component][chart].Values = append(overrides[component][chart].Values, helmVal)
	}
	return nil
}
