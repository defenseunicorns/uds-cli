// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler contains functions for interacting with, managing and deploying Zarf bundles.
package bundler

import (
	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/types"
)

const (
	// BundleYAML is the name of the bundle's metadata file
	BundleYAML = "uds-pkg.yaml"
	// BundleYAMLSignature is the name of the bundle's metadata signature file
	BundleYAMLSignature = "uds-pkg.yaml.sig"
	// BundlePrefix is the prefix for all bundle files
	BundlePrefix = "uds-pkg-"
	// PublicKeyFile is the name of the public key file
	PublicKeyFile = "public.key"
)

var (
	// BundleAlwaysPull is a list of paths that will always be pulled from the remote repository.
	BundleAlwaysPull = []string{BundleYAML, BundleYAMLSignature}
)

var defaultZarfInitOptions types.ZarfInitOptions = types.ZarfInitOptions{
	GitServer: types.GitServerInfo{
		PushUsername: config.ZarfGitPushUser,
	},
	RegistryInfo: types.RegistryInfo{
		PushUsername: config.ZarfRegistryPushUser,
	},
}
