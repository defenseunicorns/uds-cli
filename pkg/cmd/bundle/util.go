// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package util provides shared utility functions for CLI commands.
package bundle

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/printer"
	"github.com/spf13/cobra"
)

// validateBundlePathConfig holds options for ValidateBundlePath.
type validateBundlePathConfig struct {
	allowArtifactBundlePath bool
}

// ValidateBundlePathOption configures ValidateBundlePath behavior.
type ValidateBundlePathOption func(*validateBundlePathConfig)

// AllowArtifactBundlePath enables .tar.zst bundle artifact paths in ValidateBundlePath.
// Pass this to commands that support artifact deployment (e.g. deploy).
func AllowArtifactBundlePath() ValidateBundlePathOption {
	return func(c *validateBundlePathConfig) { c.allowArtifactBundlePath = true }
}

// ValidateBundlePath checks if a user-provided bundle reference is valid.
// Use this in Validate() methods. Pass AllowArtifactBundlePath() for commands that also
// accept .tar.zst bundle artifact.
//
// It checks:
//   - Empty string → error
//   - OCI references → error (not yet supported)
//   - tar.zst archives → validated for existence if AllowArtifactBundlePath(); error otherwise
//   - Path exists on filesystem
//   - If directory, contains bundle.uds.hcl
//   - If file, is named bundle.uds.hcl
//
// Returns nil if valid, or an error describing the problem.
func ValidateBundlePath(ref string, opts ...ValidateBundlePathOption) error {
	cfg := &validateBundlePathConfig{}
	for _, o := range opts {
		o(cfg)
	}

	if ref == "" {
		return fmt.Errorf("bundle file path is required")
	}

	// Check for tar.zst archive (before filesystem checks)
	if bundle.IsTarZst(ref) {
		if !cfg.allowArtifactBundlePath {
			return fmt.Errorf("tar.zst bundles are not supported for this command")
		}
		info, err := os.Stat(ref)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("bundle artifact not found: %s", ref)
			}
			return fmt.Errorf("cannot access bundle artifact %s: %w", ref, err)
		}
		if info.IsDir() {
			return fmt.Errorf("bundle artifact path is a directory: %s", ref)
		}
		return nil
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
		bundlePath := filepath.Join(ref, bundle.BundleFileName)
		if _, err := os.Stat(bundlePath); err != nil {
			return fmt.Errorf("directory does not contain %s: %s", bundle.BundleFileName, ref)
		}
		return nil
	}

	// It's a file - validate it's named bundle.uds.hcl
	if filepath.Base(ref) != bundle.BundleFileName {
		return fmt.Errorf("expected file named '%s', got: %s", bundle.BundleFileName, filepath.Base(ref))
	}

	return nil
}

// ResolvePrinter resolves the --output flag into a ResourcePrinter.
// This centralizes the printer resolution logic shared by all bundle commands.
func ResolvePrinter(cmd *cobra.Command) (printer.ResourcePrinter, error) {
	var outputFormat string
	if f := cmd.Flags().Lookup("output"); f != nil {
		outputFormat = f.Value.String()
	}
	format, err := printer.ParseFormat(outputFormat)
	if err != nil {
		return nil, err
	}
	return printer.NewPrinter(format)
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
