// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package util provides shared utility functions for CLI commands.
package bundle

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
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

// ValidateDevDeployPath validates bundle definition input and redirects artifact
// users to the production deploy command.
func ValidateDevDeployPath(ref string) error {
	info, err := os.Stat(ref)
	if err == nil {
		if info.IsDir() {
			bundlePath := filepath.Join(ref, bundle.BundleFileName)
			if _, err := os.Stat(bundlePath); err != nil {
				return fmt.Errorf("directory does not contain %s: %s", bundle.BundleFileName, ref)
			}
			return nil
		}
		if filepath.Base(ref) == bundle.BundleFileName {
			return nil
		}
		if bundle.IsTarZst(ref) {
			return fmt.Errorf("created bundle artifacts must be deployed with 'uds bundle deploy <bundle-artifact>'")
		}
		return fmt.Errorf("expected file named '%s', got: %s", bundle.BundleFileName, filepath.Base(ref))
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("cannot access bundle definition %s: %w", ref, err)
	}
	if bundle.IsTarZst(ref) || bundle.IsOCIReference(ref) {
		return fmt.Errorf("created bundle artifacts must be deployed with 'uds bundle deploy <bundle-artifact>'")
	}
	return ValidateBundlePath(ref)
}

// ValidateArtifactReference validates a local or OCI bundle artifact reference.
func ValidateArtifactReference(ref string) error {
	if ref == "" {
		return fmt.Errorf("bundle artifact is required")
	}
	if strings.HasPrefix(ref, "oci://") {
		return nil
	}
	info, err := os.Stat(ref)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("bundle definitions must be deployed with 'uds bundle dev deploy <bundle-definition>'")
		}
		if filepath.Base(ref) == bundle.BundleFileName {
			return fmt.Errorf("bundle definitions must be deployed with 'uds bundle dev deploy <bundle-definition>'")
		}
		if bundle.IsTarZst(ref) {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("local bundle artifact must be a regular file: %s", ref)
			}
			return nil
		}
		return fmt.Errorf("expected a local .tar.zst bundle artifact or OCI reference, got: %s", ref)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("cannot access bundle artifact %s: %w", ref, err)
	}
	if bundle.IsTarZst(ref) {
		return fmt.Errorf("bundle artifact not found: %s", ref)
	}
	if filepath.Base(ref) == bundle.BundleFileName {
		return fmt.Errorf("bundle definitions must be deployed with 'uds bundle dev deploy <bundle-definition>'")
	}
	if bundle.IsOCIReference(ref) {
		return nil
	}
	return fmt.Errorf("expected a local .tar.zst bundle artifact or OCI reference, got: %s", ref)
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

// PromptConfirmation writes message + " [y/N]: " to streams.ErrOut, reads one
// line from streams.In, and returns true for "y" or "yes" (case-insensitive).
// EOF and bare Enter ("unexpected newline") are treated as a "no" (false, nil).
// Any other read error is returned so callers can distinguish a real I/O failure.
func PromptConfirmation(streams iostreams.IOStreams, message string) (bool, error) {
	_, _ = fmt.Fprint(streams.ErrOut(), "\n"+message+" [y/N]: ")
	var response string
	_, err := fmt.Fscanln(streams.In(), &response)
	if err != nil {
		if errors.Is(err, io.EOF) || err.Error() == "unexpected newline" {
			return false, nil
		}
		return false, fmt.Errorf("reading confirmation: %w", err)
	}
	return strings.EqualFold(response, "y") || strings.EqualFold(response, "yes"), nil
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
