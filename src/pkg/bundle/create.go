// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"fmt"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/interactive"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/pterm/pterm"
	"helm.sh/helm/v3/pkg/chartutil"
)

// Create creates a bundle
func (b *Bundle) Create() error {

	// read the bundle's metadata into memory
	if err := utils.ReadYAMLStrict(filepath.Join(b.cfg.CreateOpts.SourceDirectory, b.cfg.CreateOpts.BundleFile), &b.bundle); err != nil {
		return err
	}

	// Populate values from valuesFiles if provided
	if err := b.processValuesFiles(); err != nil {
		return err
	}

	// Populate values from valuesFiles if provided
	if err := b.processValuesFiles(); err != nil {
		return err
	}

	// confirm creation
	if ok := b.confirmBundleCreation(); !ok {
		return fmt.Errorf("bundle creation cancelled")
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
		if err := zarfUtils.WriteYaml(bundlePath, &b.bundle, 0600); err != nil {
			return err
		}

		getSigCreatePassword := func(_ bool) ([]byte, error) {
			if b.cfg.CreateOpts.SigningKeyPassword != "" {
				return []byte(b.cfg.CreateOpts.SigningKeyPassword), nil
			}
			return interactive.PromptSigPassword()
		}
		// sign the bundle
		signaturePath := filepath.Join(b.tmp, config.BundleYAMLSignature)
		_, err := zarfUtils.CosignSignBlob(bundlePath, signaturePath, b.cfg.CreateOpts.SigningKeyPath, getSigCreatePassword)
		if err != nil {
			return err
		}
	}

	opts := bundler.Options{
		Bundle:    &b.bundle,
		Output:    b.cfg.CreateOpts.Output,
		TmpDstDir: b.tmp,
		SourceDir: b.cfg.CreateOpts.SourceDirectory,
	}
	bundlerClient := bundler.NewBundler(&opts)
	return bundlerClient.Create()
}

// confirmBundleCreation prompts the user to confirm bundle creation
func (b *Bundle) confirmBundleCreation() (confirm bool) {

	message.HeaderInfof("🎁 BUNDLE DEFINITION")
	zarfUtils.ColorPrintYAML(b.bundle, nil, false)

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
