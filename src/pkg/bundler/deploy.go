// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler contains functions for interacting with, managing and deploying UDS packages
package bundler

import (
	"context"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/packager"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
)

// Deploy deploys a bundle
//
// : create a new provider
// : pull the bundle's metadata + sig
// : read the metadata into memory
// : validate the sig (if present)
// : loop through each package
// : : load the package into a fresh temp dir
// : : validate the sig (if present)
// : : deploy the package
func (b *Bundler) Deploy() error {
	ctx := context.TODO()

	// create a new provider
	provider, err := NewProvider(ctx, b.cfg.DeployOpts.Source, b.tmp)
	if err != nil {
		return err
	}

	// pull the bundle's metadata + sig
	loaded, err := provider.LoadBundleMetadata()
	if err != nil {
		return err
	}

	// validate the sig (if present)
	if err := ValidateBundleSignature(loaded[BundleYAML], loaded[BundleYAMLSignature], b.cfg.DeployOpts.PublicKeyPath); err != nil {
		return err
	}

	// read the bundle's metadata into memory
	if err := utils.ReadYaml(loaded[BundleYAML], &b.bundle); err != nil {
		return err
	}

	// TODO: state sharing? variable scoping?

	// deploy each package
	for _, pkg := range b.bundle.ZarfPackages {
		sha := strings.Split(pkg.Ref, "@sha256:")[1]
		pkgTmp, err := utils.MakeTempDir()
		if err != nil {
			return err
		}
		defer os.RemoveAll(pkgTmp)
		_, err = provider.LoadPackage(sha, pkgTmp, config.CommonOptions.OCIConcurrency)
		if err != nil {
			return err
		}

		publicKeyPath := filepath.Join(b.tmp, PublicKeyFile)
		if pkg.PublicKey != "" {
			if err := utils.WriteFile(publicKeyPath, []byte(pkg.PublicKey)); err != nil {
				return err
			}
			defer os.Remove(publicKeyPath)
		} else {
			publicKeyPath = ""
		}

		opts := zarfTypes.ZarfPackageOptions{
			PackagePath:   pkgTmp,
			Components:    strings.Join(pkg.OptionalComponents, ","),
			PublicKeyPath: publicKeyPath,
			// TODO: SetVariables...
		}
		pkgCfg := zarfTypes.PackagerConfig{
			PkgOpts:  opts,
			InitOpts: defaultZarfInitOptions,
		}

		pkgClient, err := packager.New(&pkgCfg)
		if err != nil {
			return err
		}
		if err := pkgClient.SetTempDirectory(pkgTmp); err != nil {
			return err
		}
		if err := pkgClient.Deploy(); err != nil {
			return err
		}
	}
	return nil
}
