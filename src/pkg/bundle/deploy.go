// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/pkg/tfparser"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/uds-cli/src/types/chartvariable"
	"github.com/defenseunicorns/uds-cli/src/types/valuesources"
	goyaml "github.com/goccy/go-yaml"
	"github.com/ryboe/q"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/layout"
	"github.com/zarf-dev/zarf/src/pkg/message"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
	"golang.org/x/exp/slices"
)

// hiddenVar is the value used to mask potentially sensitive variables
const hiddenVar = "****"

// Deploy deploys a bundle
func (b *Bundle) Deploy() error {
	packagesToDeploy := b.bundle.Packages

	// Check if --packages flag is set and zarf packages have been specified
	if len(b.cfg.DeployOpts.Packages) != 0 {
		userSpecifiedPackages := strings.Split(strings.ReplaceAll(b.cfg.DeployOpts.Packages[0], " ", ""), ",")
		var selectedPackages []types.Package
		for _, pkg := range b.bundle.Packages {
			if slices.Contains(userSpecifiedPackages, pkg.Name) {
				selectedPackages = append(selectedPackages, pkg)
			}
		}

		packagesToDeploy = selectedPackages

		// Check if invalid packages were specified
		if len(userSpecifiedPackages) != len(packagesToDeploy) {
			return fmt.Errorf("invalid zarf packages specified by --packages")
		}
	}

	// if resume, filter for packages not yet deployed
	if b.cfg.DeployOpts.Resume {
		deployedPackageNames := GetDeployedPackageNames()
		var notDeployed []types.Package

		for _, pkg := range packagesToDeploy {
			if !slices.Contains(deployedPackageNames, pkg.Name) {
				notDeployed = append(notDeployed, pkg)
			}
			packagesToDeploy = notDeployed
		}
	}

	return deployPackages(packagesToDeploy, b)
}

func deployPackages(packagesToDeploy []types.Package, b *Bundle) error {
	// map of Zarf pkgs and their vars
	bundleExportedVars := make(map[string]map[string]string)

	// setup each package client and deploy
	for i, pkg := range packagesToDeploy {
		// for dev mode update package ref for remote bundles, refs for local bundles updated on create
		if config.Dev && !strings.Contains(b.cfg.DeployOpts.Source, "tar.zst") {
			pkg, err := b.setPackageRef(pkg)
			if err != nil {
				return err
			}
			b.bundle.Packages[i] = pkg
		}
		sha := strings.Split(pkg.Ref, "@sha256:")[1] // using appended SHA from create!
		pkgTmp, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
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

		pkgVars, variableData := b.loadVariables(pkg, bundleExportedVars)

		valuesOverrides, nsOverrides, err := b.loadChartOverrides(pkg, variableData)
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

		// Automatically confirm the package deployment
		zarfConfig.CommonOptions.Confirm = true

		source, err := sources.NewFromLocation(*b.cfg, pkg, opts, sha, nsOverrides)
		if err != nil {
			return err
		}

		pkgCfg := zarfTypes.PackagerConfig{
			PkgOpts:    opts,
			DeployOpts: zarfDeployOpts,
		}

		// handle zarf init configs that aren't Zarf variables
		zarfPkg, _, err := source.LoadPackageMetadata(context.TODO(), layout.New(pkgTmp), false, false)
		if err != nil {
			return err
		}

		zarfInitOpts := handleZarfInitOpts(pkgVars, zarfPkg.Kind)
		pkgCfg.InitOpts = zarfInitOpts

		pkgClient, err := packager.New(&pkgCfg, packager.WithSource(source), packager.WithTemp(opts.PackageSource))
		if err != nil {
			return err
		}

		if err = pkgClient.Deploy(context.TODO()); err != nil {
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

// handleZarfInitOpts sets the ZarfInitOptions for a package if using custom Zarf init options
func handleZarfInitOpts(pkgVars zarfVarData, zarfPkgKind v1alpha1.ZarfPackageKind) zarfTypes.ZarfInitOptions {
	if zarfPkgKind != v1alpha1.ZarfInitConfig {
		return zarfTypes.ZarfInitOptions{}
	}

	// default zarf init opts
	zarfInitOpts := zarfTypes.ZarfInitOptions{
		GitServer: zarfTypes.GitServerInfo{
			PushUsername: zarfTypes.ZarfGitPushUser,
		},
		RegistryInfo: zarfTypes.RegistryInfo{
			PushUsername: zarfTypes.ZarfRegistryPushUser,
		},
	}
	// populate zarf init opts from pkgVars
	for k, v := range pkgVars {
		switch k {
		// registry info
		case config.RegistryURL:
			zarfInitOpts.RegistryInfo.Address = v
		case config.RegistryPushUsername:
			zarfInitOpts.RegistryInfo.PushUsername = v
		case config.RegistryPushPassword:
			zarfInitOpts.RegistryInfo.PushPassword = v
		case config.RegistryPullUsername:
			zarfInitOpts.RegistryInfo.PullUsername = v
		case config.RegistryPullPassword:
			zarfInitOpts.RegistryInfo.PullPassword = v
		case config.RegistrySecretName:
			zarfInitOpts.RegistryInfo.Secret = v
		case config.RegistryNodeport:
			np, err := strconv.Atoi(v)
			if err != nil {
				message.Warnf("failed to parse nodeport %s: %v", v, err)
				return zarfTypes.ZarfInitOptions{}
			}
			zarfInitOpts.RegistryInfo.NodePort = np
		// git server info
		case config.GitURL:
			zarfInitOpts.GitServer.Address = v
		case config.GitPushUsername:
			zarfInitOpts.GitServer.PushUsername = v
		case config.GitPushPassword:
			zarfInitOpts.GitServer.PushPassword = v
		case config.GitPullUsername:
			zarfInitOpts.GitServer.PullUsername = v
		case config.GitPullPassword:
			zarfInitOpts.GitServer.PullPassword = v
		// artifact server info
		case config.ArtifactURL:
			zarfInitOpts.ArtifactServer.Address = v
		case config.ArtifactPushUsername:
			zarfInitOpts.ArtifactServer.PushUsername = v
		case config.ArtifactPushToken:
			zarfInitOpts.ArtifactServer.PushToken = v
		// storage class
		case config.StorageClass:
			zarfInitOpts.StorageClass = v
		}
	}
	return zarfInitOpts
}

// PreDeployValidationTF validates a bundle that has been built from a .tf file before deployment
// TODO: This function shares (copied) a lot of code from PreDeployValidation(). Work towards consolidating what makes sense.
func (b *Bundle) PreDeployValidationTF() (string, string, string, error) {
	// Check that provided oci source path is valid, and update it if it's missing the full path
	source, err := CheckOCISourcePath(b.cfg.DeployOpts.Source)
	if err != nil {
		return "", "", "", err
	}
	q.Q("source: pre:", source)
	b.cfg.DeployOpts.Source = source

	// create a new provider
	provider, err := NewBundleProvider(b.cfg.DeployOpts.Source, b.tmp)
	if err != nil {
		return "", "", "", err
	}

	// pull the bundles metadata
	config.BundleAlwaysPull = []string{config.BundleTF, config.BundleTFConfig, config.TerraformProvider, config.TerraformRC}
	filepaths, err := provider.LoadBundleMetadata()
	if err != nil {
		message.Warnf("unable to load the metadata of the .tf bundle: %s\n", err.Error())
		return "", "", "", err
	}

	// load the uds-bundle.tf
	message.Infof("Reading .tf at %s\n", filepaths[config.BundleTF])
	bundleTF, err := os.ReadFile(filepaths[config.BundleTF])
	if err != nil {
		return "", "", "", err
	}

	// Copy the .tf contents from the file at `/{tmpdir}/blobs/sha256/{SHASUM}` to `/{tmpdir}/main.tf`
	// This makes it a lot easier for tofu to find and use the config file
	err = os.WriteFile(filepath.Join(b.tmp, "main.tf"), bundleTF, 0o600)
	if err != nil {
		return "", "", "", err
	}

	// parse the tf config
	tfConfig, err := tfparser.ParseFile(filepaths[config.BundleTF])
	if err != nil {
		return "", "", "", err
	}
	b.tfConfig = *tfConfig

	//  read the file at conifg.BundleTFConfig and unmarshal it
	message.Infof("Reading uds-tf-config.yaml at %s\n", filepaths[config.BundleTFConfig])
	bundleTFConfigYAML, err := os.ReadFile(filepaths[config.BundleTFConfig])
	if err != nil {
		return "", "", "", err
	}

	// todo: we also read the SHAs from the uds-bundle.yaml here, should we refactor so that we use the bundle's root manifest?
	tfHelperConfig := types.TFConfigHelper{}
	if err := goyaml.Unmarshal(bundleTFConfigYAML, &tfHelperConfig); err != nil {
		return "", "", "", err
	}
	b.bundle.Packages = tfHelperConfig.Packages

	// Write a custom .terraformrc that points to the provider we brought
	customTFRC := tfparser.TerraformRC{}
	customTFRC.ProviderInstallation.Direct = make(map[string]string)
	customTFRC.ProviderInstallation.DevOverrides = make(map[string]string)
	customTFRC.ProviderInstallation.DevOverrides["defenseunicorns/uds"] = b.tmp
	err = customTFRC.WriteHCL(filepath.Join(b.tmp, config.TerraformRC))
	if err != nil {
		return "", "", "", err
	}

	// Copy the contents of the terraform provider binary into a file in the tmpdir
	providerSrc, err := os.Open(filepaths[config.TerraformProvider])
	if err != nil {
		return "", "", "", err
	}
	defer providerSrc.Close()
	providerDst, err := os.Create(filepath.Join(b.tmp, config.TerraformProvider))
	if err != nil {
		return "", "", "", err
	}
	defer providerDst.Close()
	_, err = io.Copy(providerDst, providerSrc)
	if err != nil {
		return "", "", "", err
	}
	if err := os.Chmod(filepath.Join(b.tmp, config.TerraformProvider), 0o555); err != nil {
		return "", "", "", err
	}

	// validate bundle's arch against cluster
	err = ValidateArch(config.GetArch(b.bundle.Build.Architecture))
	if err != nil {
		return "", "", "", err
	}

	// TODO: Read this metadata from the new metadata resource
	bundleConfigString := string(bundleTF)
	bundleName := "test-tf-bundle"
	return bundleName, bundleConfigString, source, err
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
	filepaths, err := provider.LoadBundleMetadata()
	if err != nil {
		return "", "", "", err
	}

	// validate the sig (if present)
	if err := ValidateBundleSignature(filepaths[config.BundleYAML], filepaths[config.BundleYAMLSignature], b.cfg.DeployOpts.PublicKeyPath); err != nil {
		return "", "", "", err
	}

	// read in file at config.BundleYAML
	message.Debugf("Reading YAML at %s", filepaths[config.BundleYAML])
	bundleYAML, err := os.ReadFile(filepaths[config.BundleYAML])
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

// ConfirmBundleDeploy prompts the user to confirm bundle creation
func (b *Bundle) ConfirmBundleDeploy() (confirm bool) {
	pkgviews := formPkgViews(b)

	message.HeaderInfof("🎁 BUNDLE DEFINITION")

	message.Title("Metadata:", "information about this bundle")
	if err := zarfUtils.ColorPrintYAML(b.bundle.Metadata, nil, false); err != nil {
		message.WarnErr(err, "unable to print bundle metadata yaml")
	}

	message.HorizontalRule()

	message.Title("Build:", "info about the machine, UDS version, and the user that created this bundle")
	if err := zarfUtils.ColorPrintYAML(b.bundle.Build, nil, false); err != nil {
		message.WarnErr(err, "unable to print bundle build yaml")
	}

	message.HorizontalRule()

	message.Title("Packages:", "definition of packages this bundle deploys, including variable overrides")

	for _, pkg := range pkgviews {
		if err := zarfUtils.ColorPrintYAML(pkg.meta, nil, false); err != nil {
			message.WarnErr(err, "unable to print package metadata yaml")
		}
		if err := zarfUtils.ColorPrintYAML(pkg.overrides, nil, false); err != nil {
			message.WarnErr(err, "unable to print package overrides yaml")
		}
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

type PkgView struct {
	meta      map[string]string
	overrides map[string]interface{}
}

// formPkgViews creates a unique pre deploy view of each package's set overrides and Zarf variables
func formPkgViews(b *Bundle) []PkgView {
	var pkgViews []PkgView
	for _, pkg := range b.bundle.Packages {
		variables := make([]interface{}, 0)

		// process variables and overrides to get values
		_, variableData := b.loadVariables(pkg, nil)
		valuesOverrides, _, _ := b.loadChartOverrides(pkg, variableData)

		for compName, component := range pkg.Overrides {
			for chartName, chart := range component {
				// filter out bundle overrides so we're left with Zarf Variables
				removeOverrides(variableData, chart.Variables)

				helmChartVars := valuesOverrides[compName][chartName]
				if helmChartVars == nil {
					continue
				}

				// takes values from helmChartVars {path: value} and form new map of {name: value}
				viewVars := extractValues(helmChartVars, chart.Variables)

				if len(viewVars) > 0 {
					variables = append(variables, map[string]map[string]interface{}{chartName: {"variables": viewVars}})
				}
			}
		}

		variables = addZarfVars(variableData, variables)
		pkgViews = append(pkgViews, PkgView{meta: formPkgMeta(pkg), overrides: map[string]interface{}{"overrides": variables}})
	}
	return pkgViews
}

func formPkgMeta(pkg types.Package) map[string]string {
	pkgMeta := map[string]string{"name": pkg.Name, "ref": pkg.Ref}
	if pkg.Repository != "" {
		pkgMeta["repo"] = pkg.Repository
	} else {
		pkgMeta["path"] = pkg.Path
	}
	return pkgMeta
}

func addZarfVars(pkgVars map[string]overrideData, variables []interface{}) []interface{} {
	for key, fv := range pkgVars {
		// "CONFIG" refers to "UDS_CONFIG" which is not a Zarf variable or override so we skip it
		if key != "CONFIG" {
			// Mask potentially secret ENV vars
			if fv.source == valuesources.Env {
				fv.value = hiddenVar
			}
			variables = append(variables, map[string]interface{}{key: fv.value})
		}
	}
	return variables
}

// extractValues returns a map of {name: value} from helmChartVars
func extractValues(helmChartVars map[string]interface{}, variables []types.BundleChartVariable) map[string]interface{} {
	viewVars := make(map[string]interface{})
	for _, v := range variables {
		// Mask potentially sensitive variables
		if v.Type == chartvariable.File || v.Source == valuesources.Env || v.Sensitive {
			viewVars[v.Name] = hiddenVar
			continue
		}

		// handle complex paths: var.helm.path = { var: { helm: { path: val } } }
		if strings.Contains(v.Path, ".") {
			paths := strings.Split(v.Path, ".")

			// set initial entry so iterations through paths can hold next key value pair until final value is found,
			// removing the entry if map[path] returns nil
			viewVars[v.Name] = helmChartVars
			for _, path := range paths {
				val, exists := viewVars[v.Name].(map[string]interface{})[path]
				if !exists {
					// delete previously set entry of v.Name and exit loop
					delete(viewVars, v.Name)
					break
				}

				viewVars[v.Name] = val
			}
		} else {
			if helmChartVars[v.Path] == nil {
				continue
			}

			viewVars[v.Name] = helmChartVars[v.Path]
		}
	}
	return viewVars
}

// removeOverrides mutates pkgVars by removing bundle overrride variables, leaving only Zarf variables
func removeOverrides(pkgVars map[string]overrideData, chartVars []types.BundleChartVariable) {
	for _, cv := range chartVars {
		// remove the bundle override variable if exists in pkgVars
		_, exists := pkgVars[strings.ToUpper(cv.Name)]
		if exists {
			delete(pkgVars, strings.ToUpper(cv.Name))
		}
	}
}
