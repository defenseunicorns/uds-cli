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
	"github.com/defenseunicorns/uds-cli/src/types"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/interactive"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/pterm/pterm"
	"helm.sh/helm/v3/pkg/chartutil"
)

// Create creates a bundle
func (b *Bundle) Create() error {

	// read the bundle's metadata into memory
	if err := utils.ReadYaml(filepath.Join(b.cfg.CreateOpts.SourceDirectory, b.cfg.CreateOpts.BundleFile), &b.bundle); err != nil {
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
		if err := utils.WriteYaml(bundlePath, &b.bundle, 0600); err != nil {
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
		_, err := utils.CosignSignBlob(bundlePath, signaturePath, b.cfg.CreateOpts.SigningKeyPath, getSigCreatePassword)
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
	utils.ColorPrintYAML(b.bundle, nil, false)

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
		for j, overrides := range pkg.Overrides {
			for k, bundleChartOverrides := range overrides {
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
						// update bundle with override values
						overrides := b.bundle.Packages[i].Overrides[j][k]
						// Merge values from valuesFile and existing values
						overrides.Values = mergeBundleChartValues(overrides.Values, valuesFileValues)
						b.bundle.Packages[i].Overrides[j][k] = overrides
					}
				}
			}
		}
	}
	return nil
}

// mergeBundleChartValues merges two lists of BundleChartValue using the values from list1 if there are duplicates
func mergeBundleChartValues(list1, list2 []types.BundleChartValue) []types.BundleChartValue {
	merged := make([]types.BundleChartValue, 0)
	paths := make(map[string]bool)

	// Add entries from list1 to the merged list
	for _, bundleChartValue := range list1 {
		merged = append(merged, bundleChartValue)
		paths[bundleChartValue.Path] = true
	}

	// Add entries from list2 to the merged list, if they don't already exist
	for _, bundleChartValue := range list2 {
		if _, ok := paths[bundleChartValue.Path]; !ok {
			merged = append(merged, bundleChartValue)
		}
	}

	return merged
}
