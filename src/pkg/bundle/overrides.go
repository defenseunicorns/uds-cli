// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

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

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/uds-cli/src/types/chartvariable"
	"github.com/defenseunicorns/uds-cli/src/types/valuesources"
	goyaml "github.com/goccy/go-yaml"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
)

// templatedVarRegex is the regex for templated variables
var templatedVarRegex = regexp.MustCompile(`\${([^}]+)}`)

type overrideData struct {
	value  interface{}
	source valuesources.Source
}
type bOverridesData map[string]overrideData
type zarfVarData map[string]string
type ZarfValues map[string]any

// loadVariables loads and sets precedence for config-level and imported variables
func (b *Bundle) loadVariables(pkg types.Package, bundleExportedVars map[string]map[string]string) (zarfVarData, bOverridesData) {
	pkgVars := make(zarfVarData)
	overVarsData := make(bOverridesData)

	// load all exported variables
	for _, exportedVarMap := range bundleExportedVars {
		for varName, varValue := range exportedVarMap {
			pkgVars[strings.ToUpper(varName)] = varValue
			overVarsData[strings.ToUpper(varName)] = overrideData{varValue, valuesources.Bundle}
		}
	}

	// Set variables in order or precedence (least specific to most specific)
	// imported vars
	for _, imp := range pkg.Imports {
		pkgVars[strings.ToUpper(imp.Name)] = bundleExportedVars[imp.Package][imp.Name]
		overVarsData[strings.ToUpper(imp.Name)] = overrideData{bundleExportedVars[imp.Package][imp.Name], valuesources.Bundle}
	}

	// shared vars
	for name, val := range b.cfg.DeployOpts.SharedVariables {
		pkgVars[strings.ToUpper(name)] = fmt.Sprint(val)
		overVarsData[strings.ToUpper(name)] = overrideData{val, valuesources.Config}
	}
	// config vars
	for name, val := range b.cfg.DeployOpts.Variables[pkg.Name] {
		pkgVars[strings.ToUpper(name)] = fmt.Sprint(val)
		overVarsData[strings.ToUpper(name)] = overrideData{val, valuesources.Config}
	}
	// env vars (vars that start with UDS_)
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(envVar, config.EnvVarPrefix) {
			parts := strings.SplitN(envVar, "=", 2)
			pkgVars[strings.ToUpper(strings.TrimPrefix(parts[0], config.EnvVarPrefix))] = parts[1]
			overVarsData[strings.ToUpper(strings.TrimPrefix(parts[0], config.EnvVarPrefix))] = overrideData{parts[1], valuesources.Env}
		}
	}
	// set vars (vars set with --set flag)
	for name, val := range b.cfg.DeployOpts.SetVariables {
		// Check if setting package specific variable (ex. packageName.variableName)
		splitName := strings.Split(name, string("."))
		if len(splitName) == 2 {
			packageName, variableName := splitName[0], splitName[1]
			if packageName == pkg.Name {
				pkgVars[strings.ToUpper(variableName)] = val
				overVarsData[strings.ToUpper(variableName)] = overrideData{val, valuesources.CLI}
			}
		} else {
			pkgVars[strings.ToUpper(name)] = val
			overVarsData[strings.ToUpper(name)] = overrideData{val, valuesources.CLI}
		}
	}
	return pkgVars, overVarsData
}

// loadChartOverrides converts a helm path to a ValuesOverridesMap config for Zarf
func (b *Bundle) loadChartOverrides(pkg types.Package, overrideData bOverridesData) (packager.ValuesOverrides, NamespaceOverrideMap, error) {
	// Create nested maps to hold the overrides
	overrideMap := make(map[string]map[string]*values.Options)
	nsOverrides := make(NamespaceOverrideMap)

	// Loop through each package component's charts and process overrides
	for componentName, component := range pkg.Overrides {
		// create component map
		overrideMap[componentName] = make(map[string]*values.Options)

		for chartName, chart := range component {
			// create chart map if overrides exist
			if len(chart.Values) > 0 || len(chart.Variables) > 0 {
				overrideMap[componentName][chartName] = &values.Options{}
			}

			overrideOpts := overrideMap[componentName][chartName]

			if err := b.processOverrideValues(overrideOpts, chart.Values, overrideData); err != nil {
				return nil, nil, err
			}
			if err := b.processOverrideVariables(overrideOpts, chart.Variables, overrideData); err != nil {
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

func resolveValuesFilePath(file string, baseDir string) (string, error) {
	if filepath.IsAbs(file) {
		if helpers.InvalidPath(file) {
			return "", fmt.Errorf("values file not found: %s", file)
		}
		return file, nil
	}

	resolved := filepath.Join(baseDir, file)
	if helpers.InvalidPath(resolved) {
		return "", fmt.Errorf("values file not found: %s (resolved to %s)", file, resolved)
	}
	return resolved, nil
}

// parseValuesFile reads and parses a YAML values file
func parseValuesFile(path string) (ZarfValues, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var vals ZarfValues
	if err := goyaml.Unmarshal(data, &vals); err != nil {
		return nil, err
	}

	return vals, nil
}

// deepMergeValues recursively merges source values into destination
func deepMergeValues(dst, src ZarfValues) {
	for key, srcVal := range src {
		if dstVal, exists := dst[key]; exists {
			srcMap, srcIsMap := srcVal.(map[string]any)
			dstMap, dstIsMap := dstVal.(map[string]any)
			if srcIsMap && dstIsMap {
				deepMergeValues(dstMap, srcMap)
			} else {
				dst[key] = srcVal
			}
		} else {
			dst[key] = srcVal
		}
	}
}

// setValueAtPath sets a value at a dot-notation path (e.g., ".app.replicas")
func setValueAtPath(vals ZarfValues, path string, value interface{}) error {
	if path == "" || !strings.HasPrefix(path, ".") {
		return fmt.Errorf("invalid path format: %s (must start with '.')", path)
	}

	// Handle root path "."
	if path == "." {
		valueMap, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("cannot merge non-map value at root path")
		}
		for k, v := range valueMap {
			vals[k] = v
		}
		return nil
	}

	// Split path into parts (remove leading dot)
	parts := strings.Split(path[1:], ".")

	// Navigate to the nested location and set the value
	current := vals
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
		} else {
			if _, exists := current[part]; !exists {
				current[part] = make(map[string]any)
			}

			nextMap, ok := current[part].(map[string]any)
			if !ok {
				return fmt.Errorf("conflict at %q, expected map but got %T", strings.Join(parts[:i+1], "."), current[part])
			}
			current = nextMap
		}
	}

	return nil
}

// getValuesVariables extracts variables with their original types preserved for use with Zarf values.
// This is needed because loadVariables converts values to strings for legacy Zarf variable compatibility,
// but Zarf values can accept complex objects (maps, arrays).
func getValuesVariables(overrideData bOverridesData) map[string]interface{} {
	result := make(map[string]interface{})
	for name, data := range overrideData {
		result[name] = data.value
	}
	return result
}

func (b *Bundle) loadPackageValues(ctx context.Context, pkg types.Package, variables map[string]interface{}) (ZarfValues, error) {
	result := make(ZarfValues)

	// Skip if no values configured
	if pkg.Values == nil && b.cfg.DeployOpts.PackageValues == nil {
		return result, nil
	}

	bundleDir := filepath.Dir(b.cfg.DeployOpts.Source)
	configDir := filepath.Dir(b.cfg.DeployOpts.Config)

	// 1. Load bundle-level values files (lowest precedence for files)
	if pkg.Values != nil {
		for _, file := range pkg.Values.Files {
			resolvedPath, err := resolveValuesFilePath(file, bundleDir)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve bundle values file %s: %w", file, err)
			}
			fileVals, err := parseValuesFile(resolvedPath)
			if err != nil {
				return nil, fmt.Errorf("failed to parse bundle values file %s: %w", resolvedPath, err)
			}
			deepMergeValues(result, fileVals)
		}

		// 2. Apply bundle-level set values
		for path, val := range pkg.Values.Set {
			if err := setValueAtPath(result, path, val); err != nil {
				return nil, fmt.Errorf("failed to set bundle value at path %s: %w", path, err)
			}
		}

		// 3. Apply bundle values.variables (resolve from UDS variables)
		// Values can be complex objects (maps, arrays) or simple values (strings, numbers)
		for _, v := range pkg.Values.Variables {
			varName := strings.ToUpper(v.Name)
			var varValue interface{}

			if val, ok := variables[varName]; ok {
				varValue = val
			} else if v.Default != nil {
				varValue = v.Default
			} else {
				continue // Skip if no value and no default
			}

			if err := setValueAtPath(result, v.Path, varValue); err != nil {
				return nil, fmt.Errorf("failed to set value variable %s at path %s: %w", v.Name, v.Path, err)
			}
		}
	}

	// 4. Load config-level values (higher precedence)
	if b.cfg.DeployOpts.PackageValues != nil {
		if pkgConfig, ok := b.cfg.DeployOpts.PackageValues[pkg.Name]; ok {
			// Load config values files
			for _, file := range pkgConfig.Files {
				resolvedPath, err := resolveValuesFilePath(file, configDir)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve config values file %s: %w", file, err)
				}
				fileVals, err := parseValuesFile(resolvedPath)
				if err != nil {
					return nil, fmt.Errorf("failed to parse config values file %s: %w", resolvedPath, err)
				}
				deepMergeValues(result, fileVals)
			}

			// Apply config set values (highest precedence)
			for path, val := range pkgConfig.Set {
				if err := setValueAtPath(result, path, val); err != nil {
					return nil, fmt.Errorf("failed to set config value at path %s: %w", path, err)
				}
			}
		}
	}

	return result, nil
}

// convertOverridesMap converts a map of overrides to a PkgOverrideMap
func convertOverridesMap(overrideMap map[string]map[string]*values.Options) (packager.ValuesOverrides, error) {
	processed := make(packager.ValuesOverrides)
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

// processOverrideNamespaces processes a bundles namespace overrides and adds them to the override map
func (b *Bundle) processOverrideNamespaces(overrideMap NamespaceOverrideMap, ns string, componentName string, chartName string) {
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
func (b *Bundle) processOverrideValues(overrideOpts *values.Options, values []types.BundleChartValue, pkgVars bOverridesData) error {
	for _, v := range values {
		// Add the override to the map, or return an error if the path is invalid
		if err := b.addOverride(overrideOpts, v, v.Value, pkgVars); err != nil {
			return err
		}
	}
	return nil
}

// processOverrideVariables processes bundle variables overrides and adds them to the override map
func (b *Bundle) processOverrideVariables(overrideOpts *values.Options, variables []types.BundleChartVariable, overrideData map[string]overrideData) error {
	for i := range variables {
		v := &variables[i]
		var overrideVal interface{}
		// Ensuring variable name is upper case since comparisons are being done against upper case env and config variables
		v.Name = strings.ToUpper(v.Name)

		overrideVal = overrideData[v.Name].value
		v.Source = overrideData[v.Name].source

		// if not found in overrideData, check for bundle default value, else was not set
		if overrideVal == nil {
			if v.Default != nil {
				overrideVal = v.Default
				v.Source = valuesources.Bundle
			} else {
				continue
			}
		}

		// Add the override to the map, or return an error if the path is invalid
		if err := b.addOverride(overrideOpts, *v, overrideVal, nil); err != nil {
			return err
		}
	}

	return nil
}

// addOverride adds a value or variable to the override map helm values options
func (b *Bundle) addOverride(overrideOpts *values.Options, override interface{}, value interface{}, pkgVars bOverridesData) error {
	var valuePath string
	// only possible for types.BundleChartValue
	var handleTemplatedVals bool

	switch v := override.(type) {
	case types.BundleChartValue:
		valuePath = v.Path
		handleTemplatedVals = true
	case types.BundleChartVariable:
		valuePath = v.Path
		handleTemplatedVals = false
		if v.Type == chartvariable.File {
			if fileVals, err := b.addFileValue(overrideOpts.FileValues, value.(string), v); err == nil {
				overrideOpts.FileValues = fileVals
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
		if handleTemplatedVals {
			jsonVals = setTemplatedVariables(jsonVals, pkgVars)
		}
		overrideOpts.JSONValues = append(overrideOpts.JSONValues, jsonVals)
	case map[string]interface{}:
		// handle objects by parsing them as json and appending to Options.JSONValues
		json, err := json.Marshal(v)
		if err != nil {
			return err
		}
		// use JSONValues because we can easily marshal the YAML to JSON and Helm understands it
		val := fmt.Sprintf("%s=%s", valuePath, json)
		if handleTemplatedVals {
			val = setTemplatedVariables(val, pkgVars)
		}
		overrideOpts.JSONValues = append(overrideOpts.JSONValues, val)
	default:
		if handleTemplatedVals {
			templatedVariable := fmt.Sprintf("%v", v)
			value = setTemplatedVariables(templatedVariable, pkgVars)
		}

		// Handle default case of simple values like strings and numbers
		helmVal := fmt.Sprintf("%s=%v", valuePath, value)
		overrideOpts.Values = append(overrideOpts.Values, helmVal)
	}
	return nil
}

// setTemplatedVariables sets the value for the templated variables
func setTemplatedVariables(templatedVariables string, pkgVars bOverridesData) string {
	// Use ReplaceAllStringFunc to handle all occurrences of templated variables
	replacedValue := templatedVarRegex.ReplaceAllStringFunc(templatedVariables, func(match string) string {
		// returns slice with the templated variable and the variable name
		variableName := templatedVarRegex.FindStringSubmatch(match)[1]
		// If we have a templated variable, get the value from pkgVars (use uppercase for case-insensitive comparison)
		if data, ok := pkgVars[strings.ToUpper(variableName)]; ok {
			return fmt.Sprint(data.value)
		}
		return fmt.Sprintf("${%s}", variableName)
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
