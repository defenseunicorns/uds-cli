// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/uds-cli/src/types/valuesources"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
)

func convertOverridesMap(overrideMap map[string]map[string]*values.Options) (PkgOverrideMap, error) {
	processed := make(PkgOverrideMap)
	// Convert the options.Values map (located in chart.MergeValues) to the PkgOverrideMap format
	for componentName, component := range overrideMap {
		componentMap := make(map[string]map[string]interface{})

		for chartName, chart := range component {
			//escape commas (with \\) in values so helm v3 can process them
			for i, value := range chart.Values {
				chart.Values[i] = strings.ReplaceAll(value, ",", "\\,")
			}
			// Merge the chart values with Helm
			data, err := chart.MergeValues(getter.Providers{})
			if err != nil {
				return nil, err
			}

			componentMap[chartName] = data
		}

		processed[componentName] = componentMap
	}
	return processed, nil
}

// loadChartOverrides converts a helm path to a ValuesOverridesMap config for Zarf
func (b *Bundle) loadChartOverrides(pkg types.Package, pkgVars map[string]string) (PkgOverrideMap, sources.NamespaceOverrideMap, error) {

	// Create nested maps to hold the overrides
	overrideMap := make(map[string]map[string]*values.Options)
	nsOverrides := make(sources.NamespaceOverrideMap)

	// Loop through each package component's charts and process overrides
	for componentName, component := range pkg.Overrides {
		for chartName, chart := range component {

			// create component and chart map
			if len(chart.Values) > 0 || len(chart.Variables) > 0 {
				overrideMap[componentName] = make(map[string]*values.Options)
				overrideMap[componentName][chartName] = &values.Options{}
			}

			if err := b.processOverrideValues(&overrideMap, chart.Values, componentName, chartName, pkgVars); err != nil {
				return nil, nil, err
			}
			if err := b.processOverrideVariables(&overrideMap, pkg.Name, chart.Variables, componentName, chartName); err != nil {
				return nil, nil, err
			}
			b.processOverrideNamespaces(nsOverrides, chart.Namespace, componentName, chartName)
		}
	}

	processed, err := convertOverridesMap(overrideMap)
	if err != nil {
		return nil, nil, err
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
func (b *Bundle) processOverrideValues(overrideMap *map[string]map[string]*values.Options, values []types.BundleChartValue, componentName string, chartName string, pkgVars map[string]string) error {
	for _, v := range values {
		// Add the override to the map, or return an error if the path is invalid
		if err := b.addOverride((*overrideMap)[componentName][chartName], v, v.Value, pkgVars); err != nil {
			return err
		}
	}
	return nil
}

type overrideView struct {
	value  string
	source valuesources.Source
}

// loadVariables loads and sets precedence for config-level and imported variables
func (b *Bundle) loadVariables(pkg types.Package, bundleExportedVars map[string]map[string]string) (map[string]string, map[string]overrideView) {

	pkgVars := make(map[string]string)
	forView := make(map[string]overrideView)

	// load all exported variables
	for _, exportedVarMap := range bundleExportedVars {
		for varName, varValue := range exportedVarMap {
			pkgVars[strings.ToUpper(varName)] = varValue
			forView[strings.ToUpper(varName)] = overrideView{varValue, valuesources.Bundle}
		}
	}

	// Set variables in order or precedence (least specific to most specific)
	// imported vars
	for _, imp := range pkg.Imports {
		pkgVars[strings.ToUpper(imp.Name)] = bundleExportedVars[imp.Package][imp.Name]
		forView[strings.ToUpper(imp.Name)] = overrideView{bundleExportedVars[imp.Package][imp.Name], valuesources.Bundle}
	}

	// shared vars
	for name, val := range b.cfg.DeployOpts.SharedVariables {
		pkgVars[strings.ToUpper(name)] = fmt.Sprint(val)
		forView[strings.ToUpper(name)] = overrideView{fmt.Sprint(val), valuesources.Config}
	}
	// config vars
	for name, val := range b.cfg.DeployOpts.Variables[pkg.Name] {
		pkgVars[strings.ToUpper(name)] = fmt.Sprint(val)
		forView[strings.ToUpper(name)] = overrideView{fmt.Sprint(val), valuesources.Config}
	}
	// env vars (vars that start with UDS_)
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(envVar, config.EnvVarPrefix) {
			parts := strings.Split(envVar, "=")
			pkgVars[strings.ToUpper(strings.TrimPrefix(parts[0], config.EnvVarPrefix))] = parts[1]
			forView[strings.ToUpper(strings.TrimPrefix(parts[0], config.EnvVarPrefix))] = overrideView{parts[1], valuesources.Env}
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
				forView[strings.ToUpper(variableName)] = overrideView{fmt.Sprint(val), valuesources.CLI}
			}
		} else {
			pkgVars[strings.ToUpper(name)] = fmt.Sprint(val)
			forView[strings.ToUpper(name)] = overrideView{fmt.Sprint(val), valuesources.CLI}
		}
	}
	return pkgVars, forView
}

// processOverrideVariables processes bundle variables overrides and adds them to the override map
func (b *Bundle) processOverrideVariables(overrideMap *map[string]map[string]*values.Options, pkgName string, variables []types.BundleChartVariable, componentName string, chartName string) error {
	for i := range variables {
		v := &variables[i]
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
		if err := b.addOverride((*overrideMap)[componentName][chartName], *v, overrideVal, nil); err != nil {
			return err
		}
	}

	return nil
}

// addOverride adds a value or variable to a PkgOverrideMap
func (b *Bundle) addOverride(overrides *values.Options, override interface{}, value interface{}, pkgVars map[string]string) error {
	var valuePath string
	var handleExports bool

	switch v := override.(type) {
	case types.BundleChartValue:
		valuePath = v.Path
		handleExports = true
	case types.BundleChartVariable:
		valuePath = v.Path
		handleExports = false
		if v.Type == types.File {
			if fileVals, err := b.addFileValue(overrides.FileValues, value.(string), v); err == nil {
				overrides.FileValues = fileVals
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
		if handleExports {
			jsonVals = setTemplatedVariables(jsonVals, pkgVars)
		}
		overrides.JSONValues = append(overrides.JSONValues, jsonVals)
	case map[string]interface{}:
		// handle objects by parsing them as json and appending to Options.JSONValues
		j, err := json.Marshal(v)
		if err != nil {
			return err
		}
		// use JSONValues because we can easily marshal the YAML to JSON and Helm understands it
		val := fmt.Sprintf("%s=%s", valuePath, j)
		if handleExports {
			val = setTemplatedVariables(val, pkgVars)
		}
		overrides.JSONValues = append(overrides.JSONValues, val)
	default:
		// Check for any templated variables if pkgVars set
		if handleExports {
			templatedVariable := fmt.Sprintf("%v", v)
			value = setTemplatedVariables(templatedVariable, pkgVars)
		}

		// Handle default case of simple values like strings and numbers
		helmVal := fmt.Sprintf("%s=%v", valuePath, value)
		overrides.Values = append(overrides.Values, helmVal)
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

	if _, err := helpers.IsTextFile(filePath); err != nil {
		return "", err
	}

	return filePath, nil
}

func filterOverrides(chartVars []types.BundleChartVariable, pkgVars map[string]overrideView) map[string]overrideView {
	pkgVarsCopy := pkgVars
	for _, cv := range chartVars {
		if pkgVarsCopy[strings.ToUpper(cv.Name)].value != "" {
			delete(pkgVarsCopy, strings.ToUpper(cv.Name))
		}
	}

	return pkgVarsCopy
}
