// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"oras.land/oras-go/v2/registry"

	"github.com/AlecAivazis/survey/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/interactive"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	"github.com/pterm/pterm"
)

// Create creates a bundle
func (b *Bundler) Create() error {
	// get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// cd into base
	if err := os.Chdir(b.cfg.CreateOpts.SourceDirectory); err != nil {
		return err
	}
	defer os.Chdir(cwd)

	// read the bundle's metadata into memory
	if err := utils.ReadYaml(config.BundleYAML, &b.bundle); err != nil {
		return err
	}

	// replace BNDL_TMPL_* variables
	if err := b.templateBundleYaml(); err != nil {
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
	if err := b.ValidateBundleResources(&b.bundle, validateSpinner); err != nil {
		return err
	}

	validateSpinner.Successf("Bundle Validated")
	pterm.Print()

	var signatureBytes []byte

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
		bytes, err := utils.CosignSignBlob(bundlePath, signaturePath, b.cfg.CreateOpts.SigningKeyPath, getSigCreatePassword)
		if err != nil {
			return err
		}
		signatureBytes = bytes
	}

	if b.cfg.CreateOpts.Output != "" {
		// set the remote's reference from the bundle's metadata
		ref, err := referenceFromMetadata(b.cfg.CreateOpts.Output, &b.bundle.Metadata, b.bundle.Metadata.Architecture)
		if err != nil {
			return err
		}
		remote, err := oci.NewOrasRemote(ref)
		if err != nil {
			return err
		}
		return CreateAndPublish(remote, &b.bundle, signatureBytes)
	}
	return Create(b, signatureBytes)
}

// adapted from p.fillActiveTemplate
func (b *Bundler) templateBundleYaml() error {
	message.Debug("Templating", config.BundleYAML, "w/:", message.JSONValue(b.cfg.CreateOpts.SetVariables))

	templateMap := map[string]string{}
	setFromCLIConfig := b.cfg.CreateOpts.SetVariables
	yamlTemplates, err := utils.FindYamlTemplates(&b.bundle, "###BNDL_TMPL_", "###")
	if err != nil {
		return err
	}

	for key := range yamlTemplates {
		_, present := setFromCLIConfig[key]
		if !present && !config.CommonOptions.Confirm {
			setVal, err := interactive.PromptVariable(zarfTypes.ZarfPackageVariable{
				Name:    key,
				Default: "",
			})

			if err == nil {
				setFromCLIConfig[key] = setVal
			} else {
				return err
			}
		} else if !present {
			return fmt.Errorf("template '%s' must be '--set' when using the '--confirm' flag", key)
		}
	}
	for key, value := range setFromCLIConfig {
		templateMap[fmt.Sprintf("###BNDL_TMPL_%s###", key)] = value
	}

	templateMap["###BNDL_ARCH###"] = b.bundle.Metadata.Architecture

	return utils.ReloadYamlTemplate(&b.bundle, templateMap)
}

// confirmBundleCreation prompts the user to confirm bundle creation
func (b *Bundler) confirmBundleCreation() (confirm bool) {

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

// copied from: https://github.com/defenseunicorns/zarf/blob/main/src/pkg/oci/utils.go
func referenceFromMetadata(registryLocation string, metadata *types.UDSMetadata, suffix string) (string, error) {
	ver := metadata.Version
	if len(ver) == 0 {
		return "", errors.New("version is required for publishing")
	}

	if !strings.HasSuffix(registryLocation, "/") {
		registryLocation = registryLocation + "/"
	}
	registryLocation = strings.TrimPrefix(registryLocation, helpers.OCIURLPrefix)

	format := "%s%s:%s-%s"

	raw := fmt.Sprintf(format, registryLocation, metadata.Name, ver, suffix)

	message.Debug("Raw OCI reference from metadata:", raw)

	ref, err := registry.ParseReference(raw)
	if err != nil {
		return "", err
	}

	return ref.String(), nil
}
