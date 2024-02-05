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
	"time"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler/fetcher"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/config/lang"
	"github.com/defenseunicorns/zarf/src/pkg/cluster"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Bundle handles bundler operations
type Bundle struct {
	// cfg is the Bundle's configuration options
	cfg *types.BundleConfig
	// bundle is the bundle's metadata read into memory
	bundle types.UDSBundle
	// tmp is the temporary directory used by the Bundle cleaned up with ClearPaths()
	tmp string
}

// New creates a new Bundle
func New(cfg *types.BundleConfig) (*Bundle, error) {
	message.Debugf("bundler.New(%s)", message.JSONValue(cfg))

	if cfg == nil {
		return nil, errors.New("bundler.New() called with nil config")
	}

	var (
		bundle = &Bundle{
			cfg: cfg,
		}
	)

	tmp, err := utils.MakeTempDir("")
	if err != nil {
		return nil, fmt.Errorf("bundler unable to create temp directory: %w", err)
	}
	bundle.tmp = tmp

	return bundle, nil
}

// NewOrDie creates a new Bundle or dies
func NewOrDie(cfg *types.BundleConfig) *Bundle {
	var (
		err    error
		bundle *Bundle
	)
	if bundle, err = New(cfg); err != nil {
		message.Fatalf(err, "bundle unable to setup, bad config: %s", err.Error())
	}
	return bundle
}

// ClearPaths clears out the paths used by Bundle
func (b *Bundle) ClearPaths() {
	_ = os.RemoveAll(b.tmp)
}

// ValidateBundleResources validates the bundle's metadata and package references
func (b *Bundle) ValidateBundleResources(bundle *types.UDSBundle, spinner *message.Spinner) error {
	// TODO: need to validate arch of local OS
	if bundle.Metadata.Architecture == "" {
		// ValidateBundle was erroneously called before CalculateBuildInfo
		if err := b.CalculateBuildInfo(); err != nil {
			return err
		}
		if bundle.Metadata.Architecture == "" {
			return errors.New("unable to determine architecture")
		}
	}
	if bundle.Metadata.Version == "" {
		return fmt.Errorf("%s is missing required field: metadata.version", config.BundleYAML)
	}
	if bundle.Metadata.Name == "" {
		return fmt.Errorf("%s is missing required field: metadata.name", config.BundleYAML)
	}

	if len(bundle.Packages) == 0 {
		return fmt.Errorf("%s is missing required list: packages", config.BundleYAML)
	}

	if err := validateBundleVars(bundle.Packages); err != nil {
		return fmt.Errorf("error validating bundle vars: %s", err)
	}

	tmp, err := utils.MakeTempDir("")
	if err != nil {
		return err
	}

	// validate access to packages as well as components referenced in the package
	for idx, pkg := range bundle.Packages {
		spinner.Updatef("Validating Bundle Package: %s", pkg.Name)
		if pkg.Name == "" {
			return fmt.Errorf("%s is missing required field: name", pkg)
		}

		if pkg.Repository == "" && pkg.Path == "" {
			return fmt.Errorf("zarf pkg %s must have either a repository or path field", pkg.Name)
		}

		if pkg.Repository != "" && pkg.Path != "" {
			return fmt.Errorf("zarf pkg %s cannot have both a repository and a path", pkg.Name)
		}

		if pkg.Ref == "" {
			return fmt.Errorf("%s .packages[%s] is missing required field: ref", config.BundleYAML, pkg.Repository)
		}
		var zarfYAML zarfTypes.ZarfPackage
		var url string
		// if using a remote repository
		// todo: refactor these hash checks using the fetcher
		if pkg.Repository != "" {
			url = fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
			if strings.Contains(pkg.Ref, "@sha256:") {
				url = fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
			}

			platform := ocispec.Platform{
				Architecture: config.GetArch(),
				OS:           oci.MultiOS,
			}
			remote, err := oci.NewOrasRemote(url, platform)
			if err != nil {
				return err
			}
			if err := remote.Repo().Reference.ValidateReferenceAsDigest(); err != nil {
				manifestDesc, _ := remote.ResolveRoot()
				// todo: don't do this here, a "validate" fn shouldn't be modifying the bundle
				bundle.Packages[idx].Ref = pkg.Ref + "@sha256:" + manifestDesc.Digest.Encoded()
			}
		} else {
			// atm we don't support outputting a bundle with local pkgs outputting to OCI
			if b.cfg.CreateOpts.Output != "" {
				return fmt.Errorf("detected local Zarf package: %s, outputting to an OCI registry is not supported when using local Zarf packages", pkg.Name)
			}
			var fullPkgName string
			if pkg.Name == "init" {
				fullPkgName = fmt.Sprintf("zarf-%s-%s-%s.tar.zst", pkg.Name, bundle.Metadata.Architecture, pkg.Ref)
			} else {
				// For local zarf packages, we get the package name using the package name provided in the bundle, since the zarf package artifact
				// uses the actual zarf package name, these names must match
				fullPkgName = fmt.Sprintf("zarf-package-%s-%s-%s.tar.zst", pkg.Name, bundle.Metadata.Architecture, pkg.Ref)
			}
			path := filepath.Join(pkg.Path, fullPkgName)
			bundle.Packages[idx].Path = path
		}

		// grab the Zarf pkg metadata
		f, err := fetcher.NewFetcher(pkg, fetcher.Config{
			PkgIter: idx, Bundle: bundle,
		})
		if err != nil {
			return err
		}
		// For local pkgs, this will throw an error if the zarf package name in the bundle doesn't match the actual zarf package name
		zarfYAML, err = f.GetPkgMetadata()
		if err != nil {
			return err
		}

		message.Debug("Validating package:", message.JSONValue(pkg))

		defer os.RemoveAll(tmp)

		// todo: need to packager.ValidatePackageSignature (or come up with a bundle-level signature scheme)
		publicKeyPath := filepath.Join(b.tmp, config.PublicKeyFile)
		if pkg.PublicKey != "" {
			if err := utils.WriteFile(publicKeyPath, []byte(pkg.PublicKey)); err != nil {
				return err
			}
			defer os.Remove(publicKeyPath)
		}

		if len(pkg.OptionalComponents) > 0 {
			// validate the optional components exist in the package and support the bundle's target architecture
			for _, component := range pkg.OptionalComponents {
				c := helpers.Find(zarfYAML.Components, func(c zarfTypes.ZarfComponent) bool {
					return c.Name == component
				})
				// make sure the component exists
				if c.Name == "" {
					return fmt.Errorf("%s .packages[%s].components[%s] does not exist in upstream: %s", config.BundleYAML, pkg.Repository, component, url)
				}
				// make sure the component supports the bundle's target architecture
				if c.Only.Cluster.Architecture != "" && c.Only.Cluster.Architecture != bundle.Metadata.Architecture {
					return fmt.Errorf("%s .packages[%s].components[%s] does not support architecture: %s", config.BundleYAML, pkg.Repository, component, bundle.Metadata.Architecture)
				}
			}
		}

		err = validateOverrides(pkg, zarfYAML)
		if err != nil {
			return err
		}

	}
	return nil
}

// CalculateBuildInfo calculates the build info for the bundle
func (b *Bundle) CalculateBuildInfo() error {
	now := time.Now()
	b.bundle.Build.User = os.Getenv("USER")

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	b.bundle.Build.Terminal = hostname

	// --architecture flag > metadata.arch > build.arch > runtime.GOARCH (default)
	b.bundle.Build.Architecture = config.GetArch(b.bundle.Metadata.Architecture, b.bundle.Build.Architecture)
	b.bundle.Metadata.Architecture = b.bundle.Build.Architecture

	b.bundle.Build.Timestamp = now.Format(time.RFC1123Z)

	b.bundle.Build.Version = config.CLIVersion

	return nil
}

// ValidateBundleSignature validates the bundle signature
func ValidateBundleSignature(bundleYAMLPath, signaturePath, publicKeyPath string) error {
	if utils.InvalidPath(bundleYAMLPath) {
		return fmt.Errorf("path for %s at %s does not exist", config.BundleYAML, bundleYAMLPath)
	}
	// The package is not signed, and no public key was provided
	if signaturePath == "" && publicKeyPath == "" {
		return nil
	}
	// The package is not signed, but a public key was provided
	if utils.InvalidPath(signaturePath) && !utils.InvalidPath(publicKeyPath) {
		return fmt.Errorf("package is not signed, but a public key was provided")
	}
	// The package is signed, but no public key was provided
	if !utils.InvalidPath(signaturePath) && utils.InvalidPath(publicKeyPath) {
		return fmt.Errorf("package is signed, but no public key was provided")
	}

	// The package is signed, and a public key was provided
	return utils.CosignVerifyBlob(bundleYAMLPath, signaturePath, publicKeyPath)
}

// GetDeployedPackages returns packages that have been deployed
func GetDeployedPackages() ([]zarfTypes.DeployedPackage, error) {
	cluster := cluster.NewClusterOrDie()
	deployedPackages, errs := cluster.GetDeployedZarfPackages()
	if len(errs) > 0 {
		return nil, lang.ErrUnableToGetPackages
	}
	return deployedPackages, nil
}

// GetDeployedPackageNames returns the names of the packages that have been deployed
func GetDeployedPackageNames() []string {
	var deployedPackageNames []string
	deployedPackages, _ := GetDeployedPackages()
	for _, pkg := range deployedPackages {
		deployedPackageNames = append(deployedPackageNames, pkg.Name)
	}
	return deployedPackageNames
}

// validateOverrides ensures that the overrides have matching components and charts in the zarf package
func validateOverrides(pkg types.Package, zarfYAML zarfTypes.ZarfPackage) error {
	for componentName, chartsValues := range pkg.Overrides {
		var foundComponent *zarfTypes.ZarfComponent
		for _, component := range zarfYAML.Components {
			if component.Name == componentName {
				foundComponent = &component
				break
			}
		}
		if foundComponent == nil {
			return fmt.Errorf("invalid override: package %q does not contain the component %q", pkg.Name, componentName)
		}

		for chartName := range chartsValues {
			var foundChart *zarfTypes.ZarfChart
			for _, chart := range foundComponent.Charts {
				if chart.Name == chartName {
					foundChart = &chart
					break
				}
			}
			if foundChart == nil {
				return fmt.Errorf("invalid override: package %q does not contain the chart %q", pkg.Name, chartName)
			}
		}
	}
	return nil
}

// validateBundleVars ensures imports and exports between Zarf pkgs match up
func validateBundleVars(packages []types.Package) error {
	exports := make(map[string]string)
	for i, pkg := range packages {
		if i == 0 && pkg.Imports != nil {
			return fmt.Errorf("first package in bundle cannot have imports")
		}
		// capture exported vars from all Zarf pkgs
		if pkg.Exports != nil {
			for _, v := range pkg.Exports {
				exports[v.Name] = pkg.Name // save off pkg.Name to check when importing
			}
		}
		// ensure imports have a matching export
		if pkg.Imports != nil {
			for _, v := range pkg.Imports {
				if _, ok := exports[v.Name]; ok && v.Package == exports[v.Name] {
					continue
				}
				return fmt.Errorf("import var %s does not have a matching export", v.Name)
			}
		}
	}
	return nil
}
