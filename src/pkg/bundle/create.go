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
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/interactive"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/pterm/pterm"
)

// Create creates a bundle
func (b *Bundle) Create() error {

	// read the bundle's metadata into memory
	if err := utils.ReadYaml(filepath.Join(b.cfg.CreateOpts.SourceDirectory, b.cfg.CreateOpts.BundleFile), &b.bundle); err != nil {
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
