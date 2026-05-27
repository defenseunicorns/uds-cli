// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// temp dir/file permissions modes (owner-only) for directories/files created inside temporary or test working areas.
	tempDirPerm fs.FileMode = 0o700
	tmpFilePerm fs.FileMode = 0o600
)

// IsTarZst reports whether s has a .tar.zst extension.
func IsTarZst(s string) bool {
	return strings.HasSuffix(s, ".tar.zst")
}

// ResolveBundlePath resolves a user-provided bundle reference to the path of
// the bundle.uds.hcl file. If ref is a directory, the bundle file inside it is
// returned; otherwise ref is returned as-is.
//
// Assumes the path has already been validated with ValidateBundlePath.
func ResolveBundlePath(ref string) string {
	info, err := os.Stat(ref)
	if err != nil {
		return ref
	}
	if info.IsDir() {
		return filepath.Join(ref, BundleFileName)
	}
	return ref
}

// TrimScheme removes the scheme from a reference name
// (e.g., "oci://ghcr.io/org/repo:tag" -> "ghcr.io/org/repo:tag")
func TrimScheme(refName string) string {
	if idx := strings.Index(refName, "://"); idx >= 0 {
		return refName[idx+3:]
	}
	return refName
}

// IsOCIReference checks if a string looks like an OCI registry reference
// (e.g., "oci://ghcr.io/org/repo:tag", "ghcr.io/org/repo:tag", or "registry.example.com/image").
func IsOCIReference(s string) bool {
	// Check for explicit oci:// scheme prefix
	if strings.HasPrefix(s, "oci://") {
		return true
	}

	// Reject other scheme prefixes (http://, https://, etc.)
	if strings.Contains(s, "://") {
		return false
	}

	// If it starts with a path separator or contains backslash, it's a file path
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.Contains(s, "\\") {
		return false
	}

	// If it has known file extensions, it's a file path
	if strings.HasSuffix(s, ".hcl") || strings.Contains(s, ".tar") || strings.HasSuffix(s, ".yaml") || strings.HasSuffix(s, ".yml") {
		return false
	}

	// Reject strings with spaces
	if strings.Contains(s, " ") {
		return false
	}

	// An OCI ref looks like: domain/path or domain/path:tag or ref@sha256:...
	// Must have both a dot (domain) and a slash (path)
	return strings.Contains(s, ".") && strings.Contains(s, "/")
}
