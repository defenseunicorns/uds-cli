// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
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
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/packager"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	"github.com/pterm/pterm"
	"golang.org/x/exp/slices"
)

// PkgOverrideMap is a map of Zarf packages -> components -> Helm charts -> values/namespace
type PkgOverrideMap map[string]map[string]map[string]interface{}

// templatedVarRegex is the regex for templated variables
var templatedVarRegex = regexp.MustCompile(`\${([^}]+)}`)

// Deploy deploys a bundle
func (b *Bundle) Deploy() error {
	packagesToDeploy := b.bundle.Packages

	// Check if --packages flag is set and zarf packages have been specified
	if len(b.cfg.DeployOpts.Packages) != 0 {
		userSpecifiedPackages := strings.Split(strings.ReplaceAll(b.cfg.DeployOpts.Packages[0], " ", ""), ",")
		selectedPackages := []types.Package{}
		for _, pkg := range b.bundle.Packages {
			if slices.Contains(userSpecifiedPackages, pkg.Name) {
				selectedPackages = append(selectedPackages, pkg)
			}
		}

		// Check if invalid packages were specified
		if len(userSpecifiedPackages) != len(packagesToDeploy) {
			return fmt.Errorf("invalid zarf packages specified by --packages")
		}

		packagesToDeploy = selectedPackages
	}

	// if resume, filter for packages not yet deployed
	if b.cfg.DeployOpts.Resume {
		deployedPackageNames := GetDeployedPackageNames()
		notDeployed := []types.Package{}

		for _, pkg := range packagesToDeploy {
			if !slices.Contains(deployedPackageNames, pkg.Name) {
				notDeployed = append(notDeployed, pkg)
			}
		}
		packagesToDeploy = notDeployed
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
		utils.ColorPrintYAML(pkg.zarfVars, nil, false)
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

type PkgView struct {
	meta      map[string]string
	zarfVars  map[string]map[string]string
	overrides map[string]interface{}
}

// formPkgViews creates a unique pre deploy view of each packages set overrides and Zarf variables
func formPkgViews(b *Bundle) []PkgView {
	var pkgViews []PkgView
	for _, pkg := range b.bundle.Packages {
		pkgMeta := map[string]string{}
		variables := make([]map[string]map[string]interface{}, 0)

		pkgMeta["name"] = pkg.Name
		if pkg.Repository != "" {
			pkgMeta["repo"] = pkg.Repository
		} else {
			pkgMeta["path"] = pkg.Path
		}
		pkgMeta["ref"] = pkg.Ref

		pkgVars := b.loadVariables(pkg, nil)
		valuesOverrides, _, _ := b.loadChartOverrides(pkg, pkgVars)

		for compName, component := range pkg.Overrides {
			for chartName, chart := range component {
				pkgVars = filterOutOverrides(chart.Variables, pkgVars)

				processedVars := valuesOverrides[compName][chartName]
				if processedVars == nil {
					continue
				}

				chartVars := map[string]interface{}{}

				for _, v := range chart.Variables {
					// handle complex paths: var.helm.path = { var: { helm: { path: val } } }
					if strings.Contains(v.Path, ".") {
						paths := strings.Split(v.Path, ".")

						// hold the next {key: value} in the chain
						chartVars[v.Name] = processedVars[paths[0]]
						for i := range paths[1:] {
							if chartVars[v.Name] == nil || chartVars[v.Name].(map[string]interface{})[paths[i+1]] == nil {
								//delete any previously set entries of var.Name
								delete(chartVars, v.Name)
								break
							}

							chartVars[v.Name] = chartVars[v.Name].(map[string]interface{})[paths[i+1]]
						}
					} else {
						if processedVars[v.Path] == nil {
							continue
						}

						chartVars[v.Name] = processedVars[v.Path]
					}
				}

				if len(chartVars) > 0 {
					variables = append(variables, map[string]map[string]interface{}{chartName: {"Variables": chartVars}})
				}
			}
		}

		pkgViews = append(pkgViews, PkgView{meta: pkgMeta, zarfVars: map[string]map[string]string{"Zarf-Variables": pkgVars}, overrides: map[string]interface{}{"Overrides": variables}})
	}
	return pkgViews
}
