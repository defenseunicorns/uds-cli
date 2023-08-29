// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authorspackage config

// Package config contains configuration strings for UDS-CLI
package config

import (
	"github.com/defenseunicorns/uds-cli/src/types"
	"runtime"
)

const (
	// ZarfYAML is the string for zarf.yaml
	ZarfYAML = "zarf.yaml"

	// BlobsDir is the string for the blobs/sha256 dir in an OCI artifact
	BlobsDir = "blobs/sha256"

	// BundleYAML is the string for zarf.yaml
	BundleYAML = "uds-bundle.yaml"

	// BundlePrefix is the prefix for compiled uds bundles
	BundlePrefix = "uds-bundle-"
)

var (
	// CommonOptions tracks user-defined values that apply across commands.
	CommonOptions types.BundlerCommonOptions

	// CLIVersion track the version of the CLI
	CLIVersion = "unset"

	// CLIArch is the computer architecture of the device executing the CLI commands
	CLIArch string

	// SkipLogFile is a flag to skip logging to a file
	SkipLogFile bool
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
