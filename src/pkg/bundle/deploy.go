// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/uds-cli/src/types/chartvariable"
	"github.com/defenseunicorns/uds-cli/src/types/valuesources"
	goyaml "github.com/goccy/go-yaml"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/state"
	zarfState "github.com/zarf-dev/zarf/src/pkg/state"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	"golang.org/x/exp/slices"
)

// hiddenVar is the value used to mask potentially sensitive variables
const hiddenVar = "****"

type NamespaceOverrideMap = map[string]map[string]string

// Deploy deploys a bundle
func (b *Bundle) Deploy(ctx context.Context) error {
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
			return errors.New("invalid zarf packages specified by --packages")
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

	return deployPackages(ctx, packagesToDeploy, b)
}

func deployPackages(ctx context.Context, packagesToDeploy []types.Package, b *Bundle) error {
	// map of Zarf pkgs and their vars
	bundleExportedVars := make(map[string]map[string]string)

	for i, pkg := range packagesToDeploy {
		// for dev mode update package ref for remote bundles, refs for local bundles updated on create
		if config.Dev && !strings.Contains(b.cfg.DeployOpts.Source, "tar.zst") {
			pkg, err := b.setPackageRef(pkg)
			if err != nil {
				return err
			}
			b.bundle.Packages[i] = pkg
		}
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

		remoteOpts := packager.RemoteOptions{
			PlainHTTP:             config.CommonOptions.Insecure,
			InsecureSkipTLSVerify: config.CommonOptions.Insecure,
		}

		sha := strings.Split(pkg.Ref, "@sha256:")[1] // using appended SHA from create!

		source, err := sources.NewFromLocation(*b.cfg, pkg, pkgTmp, publicKeyPath, config.CommonOptions.Verify, sha, nsOverrides)
		if err != nil {
			return err
		}

		filter := filters.Combine(
			filters.ForDeploy(strings.Join(pkg.OptionalComponents, ","), false),
		)

		pkgLayout, _, err := source.LoadPackage(ctx, filter)
		if err != nil {
			return err
		}

		deployOpts := packager.DeployOptions{
			Timeout:                config.HelmTimeout,
			SetVariables:           pkgVars,
			ValuesOverridesMap:     valuesOverrides,
			Retries:                b.cfg.DeployOpts.Retries,
			RemoteOptions:          remoteOpts,
			AdoptExistingResources: false,
			OCIConcurrency:         config.CommonOptions.OCIConcurrency,
			GitServer:              newGitServerInfo(pkgVars, pkgLayout.Pkg.Kind),
			RegistryInfo:           newRegistryInfo(pkgVars, pkgLayout.Pkg.Kind),
			ArtifactServer:         newArtifactServerInfo(pkgVars, pkgLayout.Pkg.Kind),
			StorageClass:           newStorageClass(pkgVars, pkgLayout.Pkg.Kind),
			IsInteractive:          !config.CommonOptions.Confirm,
		}

		bundleAnnotation := map[string]string{
			AnnotationBundleName:    b.bundle.Metadata.Name,
			AnnotationBundleVersion: b.bundle.Metadata.Version,
		}
		maps.Copy(bundleAnnotation, pkgLayout.Pkg.Metadata.Annotations)

		// Set the merged annotations back on the package
		pkgLayout.Pkg.Metadata.Annotations = bundleAnnotation

		result, err := packager.Deploy(ctx, pkgLayout, deployOpts)
		if err != nil {
			return err
		}

		err = pkgLayout.Cleanup()
		if err != nil {
			return err
		}
		// save exported vars
		pkgExportedVars := make(map[string]string)
		variableConfig := result.VariableConfig
		for _, exp := range pkg.Exports {
			// ensure if variable exists in package
			setVariable, ok := variableConfig.GetSetVariable(exp.Name)
			if !ok {
				return fmt.Errorf("cannot export variable %s because it does not exist in package %s", exp.Name, pkg.Name)
			}
			pkgExportedVars[strings.ToUpper(exp.Name)] = setVariable.Value
		}
		bundleExportedVars[pkg.Name] = pkgExportedVars

		if !pkgLayout.Pkg.IsInitConfig() {
			connectStrings := zarfState.ConnectStrings{}
			for _, comp := range result.DeployedComponents {
				for _, chart := range comp.InstalledCharts {
					for k, v := range chart.ConnectStrings {
						connectStrings[k] = v
					}
				}
			}
			message.PrintConnectStringTable(connectStrings)
		}
	}
	return nil
}

// newGitServerInfo creates a new GitServerInfo for a package if using custom Zarf init options
func newGitServerInfo(pkgVars zarfVarData, zarfPkgKind v1alpha1.ZarfPackageKind) state.GitServerInfo {
	if zarfPkgKind != v1alpha1.ZarfInitConfig {
		return state.GitServerInfo{}
	}

	// default git server info
	gitServerInfo := state.GitServerInfo{
		PushUsername: state.ZarfGitPushUser,
	}

	// populate git server info from pkgVars
	for k, v := range pkgVars {
		switch k {
		case config.GitURL:
			gitServerInfo.Address = v
		case config.GitPushUsername:
			gitServerInfo.PushUsername = v
		case config.GitPushPassword:
			gitServerInfo.PushPassword = v
		case config.GitPullUsername:
			gitServerInfo.PullUsername = v
		case config.GitPullPassword:
			gitServerInfo.PullPassword = v
		}
	}
	return gitServerInfo
}

// newRegistryInfo creates a new RegistryInfo for a package if using custom Zarf init options
func newRegistryInfo(pkgVars zarfVarData, zarfPkgKind v1alpha1.ZarfPackageKind) state.RegistryInfo {
	if zarfPkgKind != v1alpha1.ZarfInitConfig {
		return state.RegistryInfo{}
	}

	// default registry info
	registryInfo := state.RegistryInfo{
		PushUsername: state.ZarfRegistryPushUser,
		RegistryMode: state.RegistryModeNodePort,
	}

	// populate registry info from pkgVars
	for k, v := range pkgVars {
		switch k {
		// registry info
		case config.RegistryURL:
			registryInfo.Address = v
		case config.RegistryMode:
			registryInfo.RegistryMode = state.RegistryMode(v)
		case config.RegistryPushUsername:
			registryInfo.PushUsername = v
		case config.RegistryPushPassword:
			registryInfo.PushPassword = v
		case config.RegistryPullUsername:
			registryInfo.PullUsername = v
		case config.RegistryPullPassword:
			registryInfo.PullPassword = v
		case config.RegistrySecretName:
			registryInfo.Secret = v
		case config.RegistryNodeport:
			np, err := strconv.Atoi(v)
			if err != nil {
				message.Warnf("failed to parse nodeport %s: %v", v, err)
				return state.RegistryInfo{}
			}
			registryInfo.NodePort = np
		}
	}
	return registryInfo
}

// newArtifactServerInfo creates a new ArtifactServerInfo for a package if using custom Zarf init options
func newArtifactServerInfo(pkgVars zarfVarData, zarfPkgKind v1alpha1.ZarfPackageKind) state.ArtifactServerInfo {
	if zarfPkgKind != v1alpha1.ZarfInitConfig {
		return state.ArtifactServerInfo{}
	}

	// default artifact server info
	artifactServerInfo := state.ArtifactServerInfo{}

	// populate artifact server info from pkgVars
	for k, v := range pkgVars {
		switch k {
		case config.ArtifactURL:
			artifactServerInfo.Address = v
		case config.ArtifactPushUsername:
			artifactServerInfo.PushUsername = v
		case config.ArtifactPushToken:
			artifactServerInfo.PushToken = v
		}
	}
	return artifactServerInfo
}

// newStorageClass creates a new storage class for a package if using custom Zarf init options
func newStorageClass(pkgVars zarfVarData, zarfPkgKind v1alpha1.ZarfPackageKind) string {
	if zarfPkgKind != v1alpha1.ZarfInitConfig {
		return ""
	}

	// default storage class
	storageClass := ""

	// populate storage class from pkgVars
	for k, v := range pkgVars {
		switch k {
		// storage class
		case config.StorageClass:
			storageClass = v
		}
	}
	return storageClass
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
	// Built-in sensitive variables that should be sanitized
	sensitiveBuiltInVars := map[string]bool{
		config.RegistryPushUsername: true,
		config.RegistryPushPassword: true,
		config.RegistryPullUsername: true,
		config.RegistryPullPassword: true,
		config.RegistrySecretName:   true,
		config.GitPushUsername:      true,
		config.GitPushPassword:      true,
		config.GitPullUsername:      true,
		config.GitPullPassword:      true,
		config.ArtifactPushUsername: true,
		config.ArtifactPushToken:    true,
	}

	for key, fv := range pkgVars {
		// "CONFIG" refers to "UDS_CONFIG" which is not a Zarf variable or override so we skip it
		if key != "CONFIG" {
			// Mask potentially secret ENV vars or built-in sensitive variables
			if fv.source == valuesources.Env || sensitiveBuiltInVars[key] {
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
