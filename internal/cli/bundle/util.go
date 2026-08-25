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

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// validateBundlePathConfig holds options for ValidateBundlePath.
type validateBundlePathConfig struct {
	allowArtifactBundlePath     bool
	allowOCIReferenceBundlePath bool
}

func isOCIReference(s string) bool {
	return udsoci.IsOCIReference(s)
}

func isTarZst(s string) bool {
	return artifact.IsTarZst(s)
}

func resolveBundlePath(path string) string {
	return bundleinternal.ResolveBundlePath(path)
}

// ValidateBundlePathOption configures ValidateBundlePath behavior.
type ValidateBundlePathOption func(*validateBundlePathConfig)

// AllowArtifactBundlePath enables .tar.zst bundle artifact paths in ValidateBundlePath.
// Pass this to commands that support artifact deployment (e.g. deploy).
func AllowArtifactBundlePath() ValidateBundlePathOption {
	return func(c *validateBundlePathConfig) { c.allowArtifactBundlePath = true }
}

// AllowOCIReferenceBundlePath enables oci:// artifact paths in ValidateBundlePath.
// Pass this to commands that support artifact deployment (e.g. deploy).
func AllowOCIReferenceBundlePath() ValidateBundlePathOption {
	return func(c *validateBundlePathConfig) { c.allowOCIReferenceBundlePath = true }
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
		return fmt.Errorf("bundle file path is required: %w", ErrInvalidArgument)
	}

	// Check for OCI reference (before filesystem checks)
	if isOCIReference(ref) {
		if !cfg.allowOCIReferenceBundlePath {
			return fmt.Errorf("%w: %w", ErrOCINotSupported, ErrUnsupportedSource)
		}
		if err := ValidateArtifactReference(ref); err != nil {
			return fmt.Errorf("invalid OCI bundle reference %q: %w", ref, err)
		}
		return nil
	}

	// Check for tar.zst archive (before filesystem checks)
	if isTarZst(ref) {
		if !cfg.allowArtifactBundlePath {
			return fmt.Errorf("tar.zst bundles are not supported for this command: %w", ErrUnsupportedSource)
		}
		info, err := os.Stat(ref)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("bundle artifact not found: %s: %w: %w", ref, ErrPathNotFound, err)
			}
			return fmt.Errorf("cannot access bundle artifact %s: %w: %w", ref, ErrInvalidPath, err)
		}
		if info.IsDir() {
			return fmt.Errorf("bundle artifact path is a directory: %s: %w", ref, ErrInvalidPath)
		}
		return nil
	}

	// Check if the path exists
	info, err := os.Stat(ref)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("bundle path not found: %s: %w: %w", ref, ErrPathNotFound, err)
		}
		return fmt.Errorf("cannot access bundle path %s: %w: %w", ref, ErrInvalidPath, err)
	}

	if info.IsDir() {
		// If it's a directory, check if bundle.uds.hcl exists in it
		bundlePath := filepath.Join(ref, bundleFileName)
		bundleInfo, err := os.Stat(bundlePath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("directory does not contain %s: %s: %w: %w", bundleFileName, ref, ErrInvalidPath, err)
			}
			return fmt.Errorf("cannot access %s in directory %s: %w: %w", bundleFileName, ref, ErrInvalidPath, err)
		}
		if bundleInfo.IsDir() {
			return fmt.Errorf("expected %s to be a file: %s: %w", bundleFileName, ref, ErrInvalidPath)
		}
		return nil
	}

	// It's a file - validate it's named bundle.uds.hcl
	if filepath.Base(ref) != bundleFileName {
		return fmt.Errorf("expected file named '%s', got: %s: %w", bundleFileName, filepath.Base(ref), ErrInvalidPath)
	}

	return nil
}

// ValidateDevDeployPath validates bundle definition input and redirects artifact
// users to the production deploy command.
func ValidateDevDeployPath(ref string) error {
	info, err := os.Stat(ref)
	if err == nil {
		if info.IsDir() {
			bundlePath := filepath.Join(ref, bundleFileName)
			if _, err := os.Stat(bundlePath); err != nil {
				return fmt.Errorf("directory does not contain %s: %s: %w: %w", bundleFileName, ref, ErrInvalidPath, err)
			}
			return nil
		}
		if filepath.Base(ref) == bundleFileName {
			return nil
		}
		if isTarZst(ref) {
			return fmt.Errorf("created bundle artifacts must be deployed with 'uds bundle deploy <bundle-artifact>': %w", ErrUnsupportedSource)
		}
		return fmt.Errorf("expected file named '%s', got: %s: %w", bundleFileName, filepath.Base(ref), ErrInvalidPath)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("cannot access bundle definition %s: %w: %w", ref, ErrInvalidPath, err)
	}
	if isTarZst(ref) || isOCIReference(ref) {
		return fmt.Errorf("created bundle artifacts must be deployed with 'uds bundle deploy <bundle-artifact>': %w", ErrUnsupportedSource)
	}
	return ValidateBundlePath(ref)
}

// ValidateArtifactReference validates a local or OCI bundle artifact reference.
func ValidateArtifactReference(ref string) error {
	if ref == "" {
		return fmt.Errorf("bundle artifact is required: %w", ErrInvalidArgument)
	}
	if strings.HasPrefix(ref, "oci://") {
		return nil
	}
	info, err := os.Stat(ref)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("bundle definitions must be deployed with 'uds bundle dev deploy <bundle-definition>': %w", ErrUnsupportedSource)
		}
		if filepath.Base(ref) == bundleFileName {
			return fmt.Errorf("bundle definitions must be deployed with 'uds bundle dev deploy <bundle-definition>': %w", ErrUnsupportedSource)
		}
		if isTarZst(ref) {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("local bundle artifact must be a regular file: %s: %w", ref, ErrInvalidPath)
			}
			return nil
		}
		return fmt.Errorf("expected a local .tar.zst bundle artifact or OCI reference, got: %s: %w", ref, ErrUnsupportedSource)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("cannot access bundle artifact %s: %w: %w", ref, ErrInvalidPath, err)
	}
	if isOCIReference(ref) {
		return nil
	}
	if isTarZst(ref) {
		return fmt.Errorf("bundle artifact not found: %s: %w: %w", ref, ErrPathNotFound, err)
	}
	if filepath.Base(ref) == bundleFileName {
		return fmt.Errorf("bundle definitions must be deployed with 'uds bundle dev deploy <bundle-definition>': %w", ErrUnsupportedSource)
	}
	return fmt.Errorf("expected a local .tar.zst bundle artifact or OCI reference, got: %s: %w", ref, ErrUnsupportedSource)
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
		return false, fmt.Errorf("%w for prompt %q: %w", ErrReadConfirmation, message, err)
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
			return fmt.Errorf("directory does not exist: %s: %w: %w", path, ErrPathNotFound, err)
		}
		return fmt.Errorf("failed to stat directory %s: %w: %w", path, ErrInvalidPath, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("path is not a directory: %s: %w", path, ErrInvalidPath)
	}
	return nil
}
