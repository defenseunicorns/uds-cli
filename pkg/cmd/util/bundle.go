// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package util

import "strings"

// BundleFileName is the standard name for UDS bundle definition files.
const BundleFileName = "bundle.uds.hcl"

// IsOCIRef checks if a string looks like an OCI registry reference
// (e.g., "ghcr.io/org/repo:tag" or "registry.example.com/image").
// It should NOT match local file paths.
func IsOCIRef(s string) bool {
	// An explicit scheme (oci://, https://, etc.) is always an OCI/registry ref.
	if strings.Contains(s, "://") {
		return true
	}

	// If it starts with a path separator or contains backslash, it's a file path
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.Contains(s, "\\") {
		return false
	}

	// If it's the bundle filename or has file extensions, it's a file path
	if s == BundleFileName || strings.Contains(s, BundleFileName) || strings.Contains(s, ".tar") || strings.Contains(s, ".yaml") || strings.Contains(s, ".yml") {
		return false
	}

	// An OCI ref typically looks like: domain/path or domain/path:tag
	// Must not contain spaces
	if strings.Contains(s, " ") {
		return false
	}

	// Must have both a dot (in the domain) and a slash (in the path)
	return strings.Contains(s, ".") && strings.Contains(s, "/")
}
