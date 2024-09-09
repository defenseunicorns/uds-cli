// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package config contains configuration strings for UDS-CLI
package config

import (
	"runtime"
	"time"

	"github.com/defenseunicorns/uds-cli/src/types"
)

const (
	// ZarfYAML is the string for zarf.yaml
	ZarfYAML = "zarf.yaml"

	// BlobsDir is the string for the blobs/sha256 dir in an OCI artifact
	BlobsDir = "blobs/sha256"

	// BundleYAML is the string for uds-bundle.yaml
	BundleYAML = "uds-bundle.yaml"

	// BundlePrefix is the prefix for compiled uds bundles
	BundlePrefix = "uds-bundle-"

	// SBOMsTar is the sboms.tar file in a Zarf pkg
	SBOMsTar = "sboms.tar"

	// BundleSBOMTar is the name of the tarball containing the bundle's SBOM
	BundleSBOMTar = "bundle-sboms.tar"

	// BundleSBOM is the name of the untarred folder containing the bundle's SBOM
	BundleSBOM = "bundle-sboms"

	// BundleYAMLSignature is the name of the bundle's metadata signature file
	BundleYAMLSignature = "uds-bundle.yaml.sig"

	// PublicKeyFile is the name of the public key file
	PublicKeyFile = "public.key"

	// ChecksumsTxt is the name of the checksums.txt file in a Zarf pkg
	ChecksumsTxt = "checksums.txt"

	// UDSCache is the directory containing cached bundle layers
	UDSCache = ".uds-cache"

	// UDSCacheLayers is the directory in the cache containing cached bundle layers
	UDSCacheLayers = "layers"

	// EnvVarPrefix is the prefix for environment variables to override bundle helm variables
	EnvVarPrefix = "UDS_"

	// CachedLogs is a file containing cached logs
	CachedLogs = "recent-logs"

	// Special Zarf init configs, can potentially refactor after https://github.com/zarf-dev/zarf/issues/1725
	RegistryURL          = "INIT_REGISTRY_URL"
	RegistryPushUsername = "INIT_REGISTRY_PUSH_USERNAME" // #nosec G101
	RegistryPushPassword = "INIT_REGISTRY_PUSH_PASSWORD" // #nosec G101
	RegistryPullUsername = "INIT_REGISTRY_PULL_USERNAME" // #nosec G101
	RegistryPullPassword = "INIT_REGISTRY_PULL_PASSWORD" // #nosec G101
	RegistrySecretName   = "INIT_REGISTRY_SECRET"
	RegistryNodeport     = "INIT_REGISTRY_NODEPORT"
	GitURL               = "INIT_GIT_URL"
	GitPushUsername      = "INIT_GIT_PUSH_USERNAME" // #nosec G101
	GitPushPassword      = "INIT_GIT_PUSH_PASSWORD" // #nosec G101
	GitPullUsername      = "INIT_GIT_PULL_USERNAME" // #nosec G101
	GitPullPassword      = "INIT_GIT_PULL_PASSWORD" // #nosec G101
	ArtifactURL          = "INIT_ARTIFACT_URL"
	ArtifactPushUsername = "INIT_ARTIFACT_PUSH_USERNAME"
	ArtifactPushToken    = "INIT_ARTIFACT_PUSH_TOKEN"
	StorageClass         = "INIT_STORAGE_CLASS"
)

var (
	// CommonOptions tracks user-defined values that apply across commands.
	CommonOptions types.BundleCommonOptions

	// NoColor is a flag to disable color output
	NoColor bool

	// CLIVersion track the version of the CLI
	CLIVersion = "unset"

	// CLIArch is the computer architecture of the device executing the CLI commands
	CLIArch string

	// SkipLogFile is a flag to skip logging to a file
	SkipLogFile bool

	// ListTasks is a flag to print available tasks in a TaskFileLocation
	ListTasks bool

	// HelmTimeout is the default timeout for helm deploys
	HelmTimeout = 15 * time.Minute

	// Dev specifies if we are running in dev mode
	Dev = false
)

// GetArch returns the arch based on a priority list with options for overriding.
func GetArch(archs ...string) string {
	// List of architecture overrides.
	priority := append([]string{CLIArch}, archs...)

	// Find the first architecture that is specified.
	for _, arch := range priority {
		if arch != "" {
			return arch
		}
	}

	return runtime.GOARCH
}

var (
	// BundleAlwaysPull is a list of paths that will always be pulled from the remote repository.
	BundleAlwaysPull = []string{BundleYAML, BundleYAMLSignature}
)

// feature flag to enable/disable features
const (
	FF_STATE_ENABLED = false
)
