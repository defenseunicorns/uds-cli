// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler"
	"github.com/defenseunicorns/uds-cli/src/pkg/interactive"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/pterm/pterm"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	"helm.sh/helm/v3/pkg/chartutil"
)

// Create creates a bundle
func (b *Bundle) Create(ctx context.Context) error {
	// read the bundle's metadata into memory
	if err := utils.ReadYAMLStrict(filepath.Join(b.cfg.CreateOpts.SourceDirectory, b.cfg.CreateOpts.BundleFile), &b.bundle); err != nil {
		return err
	}

	// set the bundle's name and version if provided via flag
	if b.cfg.CreateOpts.Name != "" {
		b.bundle.Metadata.Name = b.cfg.CreateOpts.Name
	}
	if b.cfg.CreateOpts.Version != "" {
		b.bundle.Metadata.Version = b.cfg.CreateOpts.Version
	}

	// Populate values from valuesFiles if provided
	if err := b.processValuesFiles(); err != nil {
		return err
	}

	// confirm creation
	if ok := b.confirmBundleCreation(); !ok {
		return errors.New("bundle creation cancelled")
	}

	// make the bundle's build information
	if err := b.CalculateBuildInfo(); err != nil {
		return err
	}

	// populate Zarf config
	zarfConfig.CommonOptions.Insecure = config.CommonOptions.Insecure

	validateSpinner := message.NewProgressSpinner("Validating bundle")

	defer validateSpinner.Stop()

	// validate bundle / verify access to all repositories
	if err := b.ValidateBundleResources(validateSpinner); err != nil {
		return err
	}

	validateSpinner.Successf("Bundle Validated")
	pterm.Print()

	// sign the bundle if a signing key was provided
	if b.cfg.CreateOpts.SigningKeyPath != "" {
		// write the bundle to disk so we can sign it
		bundlePath := filepath.Join(b.tmp, config.BundleYAML)
		if err := zarfUtils.WriteYaml(bundlePath, &b.bundle, 0o600); err != nil {
			return err
		}

		getSigCreatePassword := func(_ bool) ([]byte, error) {
			if b.cfg.CreateOpts.SigningKeyPassword != "" {
				return []byte(b.cfg.CreateOpts.SigningKeyPassword), nil
			}
			return interactive.PromptSigPassword()
		}

		// sign the bundle
		signBlobOptions := zarfUtils.DefaultSignBlobOptions()
		signBlobOptions.OutputSignature = filepath.Join(b.tmp, config.BundleYAMLSignature)
		signBlobOptions.PassFunc = getSigCreatePassword
		signBlobOptions.KeyRef = b.cfg.CreateOpts.SigningKeyPath
		_, err := zarfUtils.CosignSignBlobWithOptions(ctx, bundlePath, signBlobOptions)
		if err != nil {
			return err
		}
	}

	// for dev mode update package ref for local bundles, refs for remote bundles updated on deploy
	if config.Dev && len(b.cfg.DevDeployOpts.Ref) != 0 {
		for i, pkg := range b.bundle.Packages {
			pkg, _ = b.setPackageRef(pkg)
			b.bundle.Packages[i] = pkg
		}
	}

	opts := bundler.Options{
		Bundle:    &b.bundle,
		Output:    b.cfg.CreateOpts.Output,
		TmpDstDir: b.tmp,
		SourceDir: b.cfg.CreateOpts.SourceDirectory,
	}
	bundlerClient := bundler.NewBundler(&opts)

	return bundlerClient.Create(ctx)
}

// confirmBundleCreation prompts the user to confirm bundle creation
func (b *Bundle) confirmBundleCreation() (confirm bool) {
	message.HeaderInfof("🎁 BUNDLE DEFINITION")
	if err := zarfUtils.ColorPrintYAML(b.bundle, nil, false); err != nil {
		message.WarnErr(err, "unable to print yaml")
	}

	message.HorizontalRule()
	pterm.Println()

	// Display prompt if not auto-confirmed
	if config.CommonOptions.Confirm {
		return config.CommonOptions.Confirm
	}

	prompt := &survey.Confirm{
		Message: "Create this bundle?",
	}

	if err := survey.AskOne(prompt, &confirm); err != nil || !confirm {
		// User aborted or declined, cancel the action
		return false
	}
	return true
}

// processValuesFiles reads values from valuesFiles and updates the bundle with the override values
func (b *Bundle) processValuesFiles() error {
	// Populate values from valuesFiles if provided
	for i, pkg := range b.bundle.Packages {
		// Process package-level values files (for Zarf values feature)
		if pkg.Values != nil && len(pkg.Values.Files) > 0 {
			if err := b.processPackageValuesFiles(i); err != nil {
				return err
			}
		}

		// Process chart override values files (existing behavior)
		for componentName, overrides := range pkg.Overrides {
			for chartName, bundleChartOverrides := range overrides {
				valuesFilesToMerge := make([][]types.BundleChartValue, 0)
				// Iterate over valuesFiles in reverse order to ensure subsequent value files takes precedence over previous ones
				for _, valuesFile := range bundleChartOverrides.ValuesFiles {
					// Check relative vs absolute path
					fileName := filepath.Join(b.cfg.CreateOpts.SourceDirectory, valuesFile)
					if filepath.IsAbs(valuesFile) {
						fileName = valuesFile
					}
					// read values from valuesFile
					values, err := chartutil.ReadValuesFile(fileName)
					if err != nil {
						return err
					}
					if len(values) > 0 {
						// populate BundleChartValue slice to use for merging existing values
						valuesFileValues := make([]types.BundleChartValue, 0, len(values))
						for key, value := range values {
							valuesFileValues = append(valuesFileValues, types.BundleChartValue{Path: key, Value: value})
						}
						valuesFilesToMerge = append(valuesFilesToMerge, valuesFileValues)
					}
				}
				override := b.bundle.Packages[i].Overrides[componentName][chartName]
				// add override values to the end of the list of values to merge since we want them to take precedence
				valuesFilesToMerge = append(valuesFilesToMerge, override.Values)
				override.Values = mergeBundleChartValues(valuesFilesToMerge...)
				b.bundle.Packages[i].Overrides[componentName][chartName] = override
			}
		}
	}
	return nil
}

// processPackageValuesFiles reads package-level values files and merges them into Set
// This is processed at create time so the values are embedded in the bundle
func (b *Bundle) processPackageValuesFiles(pkgIdx int) error {
	pkg := &b.bundle.Packages[pkgIdx]

	// Initialize Set map if nil
	if pkg.Values.Set == nil {
		pkg.Values.Set = make(map[string]interface{})
	}

	// Build merged values from all files (later files override earlier)
	mergedValues := make(map[string]interface{})

	for _, valuesFile := range pkg.Values.Files {
		// Resolve file path relative to bundle source directory
		fileName := filepath.Join(b.cfg.CreateOpts.SourceDirectory, valuesFile)
		if filepath.IsAbs(valuesFile) {
			fileName = valuesFile
		}

		// Read and parse the values file
		values, err := chartutil.ReadValuesFile(fileName)
		if err != nil {
			return err
		}

		// Deep merge into accumulated values
		for key, value := range values {
			mergedValues[key] = deepMergeForCreate(mergedValues[key], value)
		}
	}

	// Merge file values into Set, with existing Set values taking precedence
	// Files are processed first (lower precedence), then existing Set values override
	finalSet := make(map[string]interface{})

	// First, add all values from files using dot-notation paths
	flattenToSet(mergedValues, ".", finalSet)

	// Then, apply existing Set values (higher precedence)
	for path, val := range pkg.Values.Set {
		finalSet[path] = val
	}

	pkg.Values.Set = finalSet

	// Clear Files since they've been processed and embedded in Set
	pkg.Values.Files = nil

	return nil
}

// deepMergeForCreate recursively merges src into dst, returning the merged result
func deepMergeForCreate(dst, src interface{}) interface{} {
	if dst == nil {
		return src
	}

	dstMap, dstIsMap := dst.(map[string]interface{})
	srcMap, srcIsMap := src.(map[string]interface{})

	if dstIsMap && srcIsMap {
		for key, srcVal := range srcMap {
			dstMap[key] = deepMergeForCreate(dstMap[key], srcVal)
		}
		return dstMap
	}

	// Non-map values: src overwrites dst
	return src
}

// flattenToSet converts a nested map to dot-notation paths in the Set map
func flattenToSet(values map[string]interface{}, prefix string, set map[string]interface{}) {
	for key, value := range values {
		path := prefix + key
		set[path] = value
	}
}

// mergeBundleChartValues merges lists of BundleChartValue using the values from the last list if there are any duplicates
// such that values from the last list will take precedence over the values from previous lists
func mergeBundleChartValues(bundleChartValueLists ...[]types.BundleChartValue) []types.BundleChartValue {
	mergedMap := make(map[string]types.BundleChartValue)

	// Iterate over each list in order
	for _, bundleChartValues := range bundleChartValueLists {
		// Add entries from the current list to the merged map, overwriting any existing entries
		for _, bundleChartValue := range bundleChartValues {
			mergedMap[bundleChartValue.Path] = bundleChartValue
		}
	}

	// Convert the map to a slice
	merged := make([]types.BundleChartValue, 0, len(mergedMap))
	for _, value := range mergedMap {
		merged = append(merged, value)
	}

	return merged
}
