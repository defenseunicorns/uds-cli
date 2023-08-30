// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"github.com/defenseunicorns/uds-cli/src/config"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
)

const (
	// BundleYAMLSignature is the name of the bundle's metadata signature file
	BundleYAMLSignature = "uds-bundle.yaml.sig"
	// BundlePrefix is the prefix for all bundle files
	BundlePrefix = "uds-bundle-"
	// PublicKeyFile is the name of the public key file
	PublicKeyFile = "public.key"
)

var (
	// BundleAlwaysPull is a list of paths that will always be pulled from the remote repository.
	BundleAlwaysPull = []string{config.BundleYAML, BundleYAMLSignature}
)

// need to set these in the case of deploying a Zarf init pkg
// typically these are set as part of Zarf's Viper config, which we don't use in UDS
// could technically remove, but it doesn't hurt anything for now
var defaultZarfInitOptions = zarfTypes.ZarfInitOptions{
	GitServer: zarfTypes.GitServerInfo{
		PushUsername: zarfConfig.ZarfGitPushUser,
	},
	RegistryInfo: zarfTypes.RegistryInfo{
		PushUsername: zarfConfig.ZarfRegistryPushUser,
	},
}
