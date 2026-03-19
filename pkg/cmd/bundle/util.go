// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package util provides shared utility functions for CLI commands.
package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
)

// BundleFileName is the standard name for UDS bundle definition files.
const BundleFileName = "bundle.uds.hcl"

// ValidateBundlePath checks if a user-provided bundle reference is valid.
// Use this in Validate() methods.
//
// It checks:
//   - Empty string → error
//   - OCI references → error (not yet supported)
//   - tar.zst archives → error (not yet supported)
//   - Path exists on filesystem
//   - If directory, contains bundle.uds.hcl
//   - If file, is named bundle.uds.hcl
//
// Returns nil if valid, or an error describing the problem.
func ValidateBundlePath(ref string) error {
	if ref == "" {
		return fmt.Errorf("bundle file path is required")
	}

	// Check for tar.zst archive (before filesystem checks)
	if strings.HasSuffix(ref, ".tar.zst") {
		return fmt.Errorf("tar.zst bundles not yet supported, use a local .hcl file path or directory")
	}

	// Check for OCI reference (before filesystem checks)
	if bundle.IsOCIReference(ref) {
		return fmt.Errorf("OCI bundle references not yet supported, use a local .hcl file path or directory")
	}

	// Check if the path exists
	info, err := os.Stat(ref)
	if err != nil {
		return fmt.Errorf("bundle path not found: %s", ref)
	}

	if info.IsDir() {
		// If it's a directory, check if bundle.uds.hcl exists in it
		bundlePath := filepath.Join(ref, BundleFileName)
		if _, err := os.Stat(bundlePath); err != nil {
			return fmt.Errorf("directory does not contain %s: %s", BundleFileName, ref)
		}
		return nil
	}

	// It's a file - validate it's named bundle.uds.hcl
	if filepath.Base(ref) != BundleFileName {
		return fmt.Errorf("expected file named '%s', got: %s", BundleFileName, filepath.Base(ref))
	}

	return nil
}

// ResolveBundlePath resolves a user-provided bundle reference into
// the path to the bundle.uds.hcl file. Use this in Run() methods.
//
// It handles:
//   - Directory path → returns directory/bundle.uds.hcl
//   - File path → returns the path as-is
//
// This function assumes the path has already been validated with ValidateBundlePath.
// If the path is invalid, behavior is undefined.
func ResolveBundlePath(ref string) string {
	info, err := os.Stat(ref)
	if err != nil {
		// Path doesn't exist - return as-is (caller should have validated)
		return ref
	}

	if info.IsDir() {
		return filepath.Join(ref, BundleFileName)
	}

	return ref
}

// IsTarZst checks if a string ends with .tar.zst extension.
func IsTarZst(s string) bool {
	return strings.HasSuffix(s, ".tar.zst")
}

// ValidateConfigOptions checks that the ConfigOptions has valid values.
func ValidateConfigOptions(opts bundle.ConfigOptions) error {
	if opts.Concurrency < 1 {
		return fmt.Errorf("concurrency must be >= 1, got %d", opts.Concurrency)
	}
	if err := ValidateDir(opts.TmpDir); err != nil {
		return fmt.Errorf("--tmp-dir: %w", err)
	}
	return nil
}

// ValidateDir checks that the given path exists and is a directory.
func ValidateDir(path string) error {
	if path == "" {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", path)
		}
		return fmt.Errorf("failed to stat directory %s: %w", path, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}
	return nil
}
