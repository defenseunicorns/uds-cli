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
	"regexp"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/uds-cli/src/types/valuesources"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/packager"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	"github.com/pterm/pterm"
	"golang.org/x/exp/slices"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
)

// PkgOverrideMap is a map of Zarf packages -> components -> Helm charts -> values/namespace
type PkgOverrideMap map[string]map[string]map[string]interface{}

// templatedVarRegex is the regex for templated variables
var templatedVarRegex = regexp.MustCompile(`\${([^}]+)}`)

// Deploy deploys a bundle
func (b *Bundle) Deploy() error {
	resume := b.cfg.DeployOpts.Resume

	// Check if --packages flag is set and zarf packages have been specified
	var packagesToDeploy []types.Package
	if len(b.cfg.DeployOpts.Packages) != 0 {
		userSpecifiedPackages := strings.Split(strings.ReplaceAll(b.cfg.DeployOpts.Packages[0], " ", ""), ",")

		for _, pkg := range b.bundle.Packages {
			if slices.Contains(userSpecifiedPackages, pkg.Name) {
				packagesToDeploy = append(packagesToDeploy, pkg)
			}
		}

		// Check if invalid packages were specified
		if len(userSpecifiedPackages) != len(packagesToDeploy) {
			return fmt.Errorf("invalid zarf packages specified by --packages")
		}
	} else {
		packagesToDeploy = b.bundle.Packages
	}
	return deployPackages(packagesToDeploy, resume, b)
}

func deployPackages(packages []types.Package, resume bool, b *Bundle) error {
	// map of Zarf pkgs and their vars
	bundleExportedVars := make(map[string]map[string]string)

	var packagesToDeploy []types.Package

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

	// setup each package client and deploy
	for i, pkg := range packagesToDeploy {
		// for dev mode update package ref for remote bundles, refs for local bundles updated on create
		if config.Dev && !strings.Contains(b.cfg.DeployOpts.Source, "tar.zst") {
			pkg = b.setPackageRef(pkg)
			b.bundle.Packages[i] = pkg
		}
		sha := strings.Split(pkg.Ref, "@sha256:")[1] // using appended SHA from create!
		pkgTmp, err := utils.MakeTempDir(config.CommonOptions.TempDirectory)
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

		pkgVars := b.loadVariables(pkg, bundleExportedVars)

		valuesOverrides, nsOverrides, err := b.loadChartOverrides(pkg, pkgVars)
		if err != nil {
			return err
		}

		opts := zarfTypes.ZarfPackageOptions{
			PackageSource:      pkgTmp,
			OptionalComponents: strings.Join(pkg.OptionalComponents, ","),
			PublicKeyPath:      publicKeyPath,
			SetVariables:       pkgVars,
			Retries:            b.cfg.DeployOpts.Retries,
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

		// Automatically confirm the package deployment
		zarfConfig.CommonOptions.Confirm = true

		source, err := sources.New(*b.cfg, pkg, opts, sha, nsOverrides)
		if err != nil {
			return err
		}

		pkgClient := packager.NewOrDie(&pkgCfg, packager.WithSource(source), packager.WithTemp(opts.PackageSource))
		if err := pkgClient.Deploy(context.TODO()); err != nil {
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

// loadVariables loads and sets precedence for config-level and imported variables
func (b *Bundle) loadVariables(pkg types.Package, bundleExportedVars map[string]map[string]string) map[string]string {
	pkgVars := make(map[string]string)

	// load all exported variables
	for _, exportedVarMap := range bundleExportedVars {
		for varName, varValue := range exportedVarMap {
			pkgVars[strings.ToUpper(varName)] = varValue
		}
	}

	// Set variables in order or precedence (least specific to most specific)
	// imported vars
	for _, imp := range pkg.Imports {
		pkgVars[strings.ToUpper(imp.Name)] = bundleExportedVars[imp.Package][imp.Name]
	}

	// shared vars
	for name, val := range b.cfg.DeployOpts.SharedVariables {
		pkgVars[strings.ToUpper(name)] = fmt.Sprint(val)
	}
	// config vars
	for name, val := range b.cfg.DeployOpts.Variables[pkg.Name] {
		pkgVars[strings.ToUpper(name)] = fmt.Sprint(val)
	}
	// env vars (vars that start with UDS_)
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(envVar, config.EnvVarPrefix) {
			parts := strings.Split(envVar, "=")
			pkgVars[strings.ToUpper(strings.TrimPrefix(parts[0], config.EnvVarPrefix))] = parts[1]
		}
	}
	// set vars (vars set with --set flag)
	for name, val := range b.cfg.DeployOpts.SetVariables {
		// Check if setting package specific variable (ex. packageName.variableName)
		splitName := strings.Split(name, string("."))
		if len(splitName) == 2 {
			packageName, variableName := splitName[0], splitName[1]
			if packageName == pkg.Name {
				pkgVars[strings.ToUpper(variableName)] = fmt.Sprint(val)
			}
		} else {
			pkgVars[strings.ToUpper(name)] = fmt.Sprint(val)
		}
	}
	return pkgVars
}

// loadChartOverrides converts a helm path to a ValuesOverridesMap config for Zarf
func (b *Bundle) loadChartOverrides(pkg types.Package, pkgVars map[string]string) (PkgOverrideMap, sources.NamespaceOverrideMap, error) {

	// Create nested maps to hold the overrides
	overrideMap := make(map[string]map[string]*values.Options)
	nsOverrides := make(sources.NamespaceOverrideMap)

	// Loop through each package component's charts and process overrides
	for componentName, component := range pkg.Overrides {
		for chartName, chart := range component {
			chartCopy := chart // Create a copy of the chart
			err := b.processOverrideValues(&overrideMap, &chartCopy.Values, componentName, chartName, pkgVars)
			if err != nil {
				return nil, nil, err
			}
			err = b.processOverrideVariables(&overrideMap, pkg.Name, &chartCopy.Variables, componentName, chartName)
			if err != nil {
				return nil, nil, err
			}
			b.processOverrideNamespaces(nsOverrides, chartCopy.Namespace, componentName, chartName)
		}
	}

	processed := make(PkgOverrideMap)

	// Convert the options.Values map (located in chart.MergeValues) to the PkgOverrideMap format
	for componentName, component := range overrideMap {
		// Create a map to hold all the charts in the component
		componentMap := make(map[string]map[string]interface{})

		// Loop through each chart in the component
		for chartName, chart := range component {
			//escape commas (with \\) in values so helm v3 can process them
			for i, value := range chart.Values {
				chart.Values[i] = strings.ReplaceAll(value, ",", "\\,")
			}
			// Merge the chart values with Helm
			data, err := chart.MergeValues(getter.Providers{})
			if err != nil {
				return nil, nil, err
			}

			// Add the chart values to the component map
			componentMap[chartName] = data
		}

		// Add the component map to the processed map
		processed[componentName] = componentMap
	}

	return processed, nsOverrides, nil
}

// processOverrideNamespaces processes a bundles namespace overrides and adds them to the override map
func (b *Bundle) processOverrideNamespaces(overrideMap sources.NamespaceOverrideMap, ns string, componentName string, chartName string) {
	if ns == "" {
		return // no namespace override
	}
	// check if component exists in override map
	if _, ok := overrideMap[componentName]; !ok {
		overrideMap[componentName] = make(map[string]string)
	}
	overrideMap[componentName][chartName] = ns
}

// processOverrideValues processes a bundles values overrides and adds them to the override map
func (b *Bundle) processOverrideValues(overrideMap *map[string]map[string]*values.Options, values *[]types.BundleChartValue, componentName string, chartName string, pkgVars map[string]string) error {
	for _, v := range *values {
		// Add the override to the map, or return an error if the path is invalid
		if err := b.addOverride(*overrideMap, componentName, chartName, v, v.Value, pkgVars); err != nil {
			return err
		}
	}
	return nil
}

// processOverrideVariables processes bundle variables overrides and adds them to the override map
func (b *Bundle) processOverrideVariables(overrideMap *map[string]map[string]*values.Options, pkgName string, variables *[]types.BundleChartVariable, componentName string, chartName string) error {
	for _, v := range *variables {
		var overrideVal interface{}
		// Ensuring variable name is upper case since comparisons are being done against upper case env and config variables
		v.Name = strings.ToUpper(v.Name)

		// check for override in --set vars
		for k, val := range b.cfg.DeployOpts.SetVariables {
			if strings.Contains(k, ".") {
				// check for <pkg>.<var> syntax was used in --set and use uppercase for a non-case-sensitive comparison
				setVal := strings.Split(k, ".")
				if setVal[0] == pkgName && strings.ToUpper(setVal[1]) == v.Name {
					overrideVal = val
					v.Source = valuesources.CLI
				}
			} else if strings.ToUpper(k) == v.Name {
				overrideVal = val
				v.Source = valuesources.CLI
			}
		}

		// check for override in env vars if not in --set
		if envVarOverride, exists := os.LookupEnv(strings.ToUpper(config.EnvVarPrefix + v.Name)); overrideVal == nil && exists {
			overrideVal = envVarOverride
			v.Source = valuesources.Env
		}

		// if not in --set or an env var, use the following precedence: configFile, sharedConfig, default
		if overrideVal == nil {
			if configFileOverride, existsInConfig := b.cfg.DeployOpts.Variables[pkgName][v.Name]; existsInConfig {
				overrideVal = configFileOverride
				v.Source = valuesources.Config
			} else if sharedConfigOverride, existsInSharedConfig := b.cfg.DeployOpts.SharedVariables[v.Name]; existsInSharedConfig {
				overrideVal = sharedConfigOverride
				v.Source = valuesources.Config
			} else if v.Default != nil {
				overrideVal = v.Default
				v.Source = valuesources.Bundle
			} else {
				continue
			}
		}

		// Add the override to the map, or return an error if the path is invalid
		if err := b.addOverride(*overrideMap, componentName, chartName, v, overrideVal, nil); err != nil {
			return err
		}
	}

	return nil
}

// addOverride adds a value or variable to a PkgOverrideMap
func (b *Bundle) addOverride(overrides map[string]map[string]*values.Options, component string, chart string, override interface{}, value interface{}, pkgVars map[string]string) error {
	// Create the component map if it doesn't exist
	if _, ok := overrides[component]; !ok {
		overrides[component] = make(map[string]*values.Options)
	}

	// Create the chart map if it doesn't exist
	if _, ok := overrides[component][chart]; !ok {
		overrides[component][chart] = &values.Options{}
	}

	var valuePath string

	switch v := any(override).(type) {
	case types.BundleChartValue:
		valuePath = v.Path
	case types.BundleChartVariable:
		valuePath = v.Path
		if v.Type == types.File {
			if fileVals, err := b.addFileValue(overrides[component][chart].FileValues, value.(string), v); err == nil {
				overrides[component][chart].FileValues = fileVals
			} else {
				return err
			}
			return nil
		}
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
			jsonStrs[i] = string(j)
		}
		// use JSONValues because we can easily marshal the YAML to JSON and Helm understands it
		jsonVals := fmt.Sprintf("%s=[%s]", valuePath, strings.Join(jsonStrs, ","))
		if pkgVars != nil {
			jsonVals = setTemplatedVariables(jsonVals, pkgVars)
		}
		overrides[component][chart].JSONValues = append(overrides[component][chart].JSONValues, jsonVals)
	case map[string]interface{}:
		// handle objects by parsing them as json and appending to Options.JSONValues
		j, err := json.Marshal(v)
		if err != nil {
			return err
		}
		// use JSONValues because we can easily marshal the YAML to JSON and Helm understands it
		val := fmt.Sprintf("%s=%s", valuePath, j)
		if pkgVars != nil {
			val = setTemplatedVariables(val, pkgVars)
		}
		overrides[component][chart].JSONValues = append(overrides[component][chart].JSONValues, val)
	default:
		// Check for any templated variables if pkgVars set
		if pkgVars != nil {
			templatedVariable := fmt.Sprintf("%v", v)
			value = setTemplatedVariables(templatedVariable, pkgVars)
		}

		// Handle default case of simple values like strings and numbers
		helmVal := fmt.Sprintf("%s=%v", valuePath, value)
		overrides[component][chart].Values = append(overrides[component][chart].Values, helmVal)
	}
	return nil
}

// setTemplatedVariables sets the value for the templated variables
func setTemplatedVariables(templatedVariables string, pkgVars map[string]string) string {
	// Use ReplaceAllStringFunc to handle all occurrences of templated variables
	replacedValue := templatedVarRegex.ReplaceAllStringFunc(templatedVariables, func(match string) string {
		// returns slice with the templated variable and the variable name
		variableName := templatedVarRegex.FindStringSubmatch(match)[1]
		// If we have a templated variable, get the value from pkgVars (use uppercase for case-insensitive comparison)
		if varValue, ok := pkgVars[strings.ToUpper(variableName)]; ok {
			return varValue
		}
		return fmt.Sprintf("${%s_not_found}", variableName)
	})
	return replacedValue
}

// addFileValue adds a key=filepath string to helm FileValues
func (b *Bundle) addFileValue(helmFileVals []string, filePath string, override types.BundleChartVariable) ([]string, error) {
	verifiedPath, err := formFilePath(getSourcePath(override.Source, b), filePath)
	if err != nil {
		return nil, err
	}
	helmVal := fmt.Sprintf("%s=%v", override.Path, verifiedPath)
	return append(helmFileVals, helmVal), nil
}

// getSourcePath returns the path from where a value is set
func getSourcePath(pathType valuesources.Source, b *Bundle) string {
	var sourcePath string
	switch pathType {
	case valuesources.CLI:
		sourcePath, _ = os.Getwd()
	case valuesources.Env:
		sourcePath, _ = os.Getwd()
	case valuesources.Bundle:
		sourcePath = filepath.Dir(b.cfg.DeployOpts.Source)
	case valuesources.Config:
		sourcePath = filepath.Dir(b.cfg.DeployOpts.Config)
	}

	return sourcePath
}

// formFilePath merges relative paths together to form full path and checks if the file exists
func formFilePath(anchorPath string, filePath string) (string, error) {
	if !filepath.IsAbs(filePath) {
		// set path relative to anchorPath (i.e. cwd or config), unless they are the same
		if anchorPath != filepath.Dir(filePath) {
			filePath = filepath.Join(anchorPath, filePath)
		}
	}

	if helpers.InvalidPath(filePath) {
		return "", fmt.Errorf("unable to find file %s", filePath)
	}

	_, err := helpers.IsTextFile(filePath)
	if err != nil {
		return "", err
	}

	return filePath, nil
}

// PreDeployValidation validates the bundle before deployment
func (b *Bundle) PreDeployValidation() (string, string, string, error) {

	// Check that provided oci source path is valid, and update it if it's missing the full path
	source, err := CheckOCISourcePath(b.cfg.DeployOpts.Source)
	if err != nil {
		return "", "", "", err
	}
	b.cfg.DeployOpts.Source = source

	// create a new provider
	provider, err := NewBundleProvider(b.cfg.DeployOpts.Source, b.tmp)
	if err != nil {
		return "", "", "", err
	}

	// pull the bundle's metadata + sig
	loaded, err := provider.LoadBundleMetadata()
	if err != nil {
		return "", "", "", err
	}

	// validate the sig (if present)
	if err := ValidateBundleSignature(loaded[config.BundleYAML], loaded[config.BundleYAMLSignature], b.cfg.DeployOpts.PublicKeyPath); err != nil {
		return "", "", "", err
	}

	// read in file at config.BundleYAML
	message.Debugf("Reading YAML at %s", loaded[config.BundleYAML])
	bundleYAML, err := os.ReadFile(loaded[config.BundleYAML])
	if err != nil {
		return "", "", "", err
	}

	// todo: we also read the SHAs from the uds-bundle.yaml here, should we refactor so that we use the bundle's root manifest?
	if err := goyaml.Unmarshal(bundleYAML, &b.bundle); err != nil {
		return "", "", "", err
	}

	// validate bundle's arch against cluster
	err = ValidateArch(config.GetArch(b.bundle.Build.Architecture))
	if err != nil {
		return "", "", "", err
	}

	bundleName := b.bundle.Metadata.Name
	return bundleName, string(bundleYAML), source, err
}

type PkgView struct {
	meta      map[string]string
	overrides map[string]interface{}
}

// ConfirmBundleDeploy prompts the user to confirm bundle creation
func (b *Bundle) ConfirmBundleDeploy() (confirm bool) {

	pkgviews := formPkgViews(b)

	message.HeaderInfof("🎁 BUNDLE DEFINITION")
	pterm.Println("kind: UDS Bundle")

	message.HorizontalRule()

	message.Title("Metatdata:", "information about this bundle")
	utils.ColorPrintYAML(b.bundle.Metadata, nil, false)

	message.HorizontalRule()

	message.Title("Build:", "info about the machine, UDS version, and the user that created this bundle")
	utils.ColorPrintYAML(b.bundle.Build, nil, false)

	message.HorizontalRule()

	message.Title("Packages:", "definition of packages this bundle deploys, including variable overrides")

	for _, pkg := range pkgviews {
		utils.ColorPrintYAML(pkg.meta, nil, false)
		utils.ColorPrintYAML(pkg.overrides, nil, false)
	}

	message.HorizontalRule()

	// Display prompt if not auto-confirmed
	if config.CommonOptions.Confirm {
		return config.CommonOptions.Confirm
	}

	prompt := &survey.Confirm{
		Message: "Deploy this bundle?",
	}

	if err := survey.AskOne(prompt, &confirm); err != nil || !confirm {
		return false
	}
	return true
}

// formPkgViews creates a unique pre deploy view of each packages override variables
func formPkgViews(b *Bundle) []PkgView {
	var pkgViews []PkgView
	for _, pkg := range b.bundle.Packages {
		pkgMeta := make(map[string]string)
		variables := make([]map[string]interface{}, 0)

		pkgMeta["name"] = pkg.Name
		if pkg.Repository != "" {
			pkgMeta["repo"] = pkg.Repository
		} else {
			pkgMeta["path"] = pkg.Path
		}
		pkgMeta["ref"] = pkg.Ref

		valuesOverrides, _, _ := b.loadChartOverrides(pkg, make(map[string]string))

		for compName, component := range pkg.Overrides {
			for chartName, chart := range component {
				processedVars := valuesOverrides[compName][chartName]

				for _, v := range chart.Variables {
					varMap := make(map[string]interface{})

					// handle complex paths: var.helm.path = { var: { helm: { path: val } } }
					if strings.Contains(v.Path, ".") {
						paths := strings.Split(v.Path, ".")

						// hold the next {key: value} in the chain
						val := processedVars[paths[0]]
						for i := range paths[1:] {
							if val == nil {
								val = "not set"
								break
							}
							val = val.(map[string]interface{})[paths[i+1]]
						}

						varMap[v.Path] = val
					} else {
						if processedVars[v.Path] == nil {
							varMap[v.Path] = "not set"
						} else {
							varMap[v.Path] = processedVars[v.Path]
						}

					}

					variables = append(variables, varMap)
				}
			}
		}

		pkgViews = append(pkgViews, PkgView{pkgMeta, map[string]interface{}{"Overrides": map[string][]map[string]interface{}{"Variables": variables}}})
	}
	return pkgViews
}
