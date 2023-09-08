// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"errors"
	"fmt"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/packager"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
)

// Bundler handles bundler operations
type Bundler struct {
	// cfg is the Bundler's configuration options
	cfg *types.BundlerConfig
	// bundle is the bundle's metadata read into memory
	bundle types.UDSBundle
	// tmp is the temporary directory used by the Bundler cleaned up with ClearPaths()
	tmp string
}

// New creates a new Bundler
func New(cfg *types.BundlerConfig) (*Bundler, error) {
	message.Debugf("bundler.New(%s)", message.JSONValue(cfg))

	if cfg == nil {
		return nil, errors.New("bundler.New() called with nil config")
	}

	var (
		bundler = &Bundler{
			cfg: cfg,
		}
	)

	tmp, err := utils.MakeTempDir()
	if err != nil {
		return nil, fmt.Errorf("bundler unable to create temp directory: %w", err)
	}
	bundler.tmp = tmp

	return bundler, nil
}

// NewOrDie creates a new Bundler or dies
func NewOrDie(cfg *types.BundlerConfig) *Bundler {
	var (
		err     error
		bundler *Bundler
	)
	if bundler, err = New(cfg); err != nil {
		message.Fatalf(err, "bundler unable to setup, bad config: %s", err.Error())
	}
	return bundler
}

// ClearPaths clears out the paths used by Bundler
func (b *Bundler) ClearPaths() {
	_ = os.RemoveAll(b.tmp)
	_ = os.RemoveAll(zarfConfig.ZarfSBOMDir)
}

// ValidateBundleResources validates the bundle's metadata and package references
func (b *Bundler) ValidateBundleResources(bundle *types.UDSBundle) error {
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

	if len(bundle.ZarfPackages) == 0 {
		return fmt.Errorf("%s is missing required list: packages", config.BundleYAML)
	}

	if err := validateBundleVars(bundle.ZarfPackages); err != nil {
		return fmt.Errorf("error validating bundle vars: %s", err)
	}

	tmp, err := utils.MakeTempDir()
	if err != nil {
		return err
	}

	// validate access to packages as well as components referenced in the package
	for idx, pkg := range bundle.ZarfPackages {
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
		zarfYAML := zarfTypes.ZarfPackage{}
		var url string
		// if using a remote repository
		if pkg.Repository != "" {
			url = fmt.Sprintf("%s:%s-%s", pkg.Repository, pkg.Ref, bundle.Metadata.Architecture)
			if strings.Contains(pkg.Ref, "@sha256:") {
				url = fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
			}
			remotePkg, err := bundler.NewRemoteBundler(pkg, url, nil, nil)
			if err != nil {
				return err
			}
			if err := remotePkg.RemoteSrc.Repo().Reference.ValidateReferenceAsDigest(); err != nil {
				manifestDesc, _ := remotePkg.RemoteSrc.ResolveRoot()
				bundle.ZarfPackages[idx].Ref = pkg.Ref + "-" + bundle.Metadata.Architecture + "@sha256:" + manifestDesc.Digest.Encoded()
			}
			zarfYAML, err = remotePkg.GetMetadata(url, tmp)
			if err != nil {
				return err
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
				fullPkgName = fmt.Sprintf("zarf-package-%s-%s-%s.tar.zst", pkg.Name, bundle.Metadata.Architecture, pkg.Ref)
			}
			path := filepath.Join(pkg.Path, fullPkgName)
			bundle.ZarfPackages[idx].Path = path
			p := bundler.NewLocalBundler(pkg.Path, tmp)
			if err != nil {
				return err
			}
			zarfYAML, err = p.GetMetadata(path, tmp)
			if err != nil {
				return err
			}
		}

		message.Debug("Validating package:", message.JSONValue(pkg))

		defer os.RemoveAll(tmp)

		publicKeyPath := filepath.Join(b.tmp, config.PublicKeyFile)
		if pkg.PublicKey != "" {
			if err := utils.WriteFile(publicKeyPath, []byte(pkg.PublicKey)); err != nil {
				return err
			}
			defer os.Remove(publicKeyPath)
		} else {
			publicKeyPath = ""
		}

		if err := packager.ValidatePackageSignature(tmp, publicKeyPath); err != nil {
			return err
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
	}
	return nil
}

// validateBundleVars ensures imports and exports between Zarf pkgs match up
func validateBundleVars(packages []types.BundleZarfPackage) error {
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

// CalculateBuildInfo calculates the build info for the bundle
//
// this is mainly mirrored from packager.writeYaml()
func (b *Bundler) CalculateBuildInfo() error {
	now := time.Now()
	b.bundle.Build.User = os.Getenv("USER")

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	b.bundle.Build.Terminal = hostname

	// --architecture flag > metadata.arch > build.arch / runtime.GOARCH (default)
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
