// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
	"os"

	"github.com/defenseunicorns/uds-cli/internal/logger"
)

// ValidateConfig is the single entry point for validating a fully-resolved
// UDSBundleConfig. It runs nil-checks on the structure and then delegates
// field-level checks to focused sub-validators.
//
// Call this once at the boundary where config is produced (e.g. from the
// ConfigResolver). Downstream consumers should trust the config and skip
// re-validation.
func ValidateConfig(cfg *UDSBundleConfig) error {
	if err := validateConfigStructure(cfg); err != nil {
		return err
	}
	return validateOptions(cfg.Options)
}

// validateConfigStructure asserts that cfg and its required sub-structs are non-nil.
func validateConfigStructure(cfg *UDSBundleConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is required (UDSBundleConfig must not be nil): %w", ErrConfigRequired)
	}
	if cfg.Options == nil {
		return fmt.Errorf("config.Options is required (ConfigOptions must not be nil): %w", ErrConfigOptionsRequired)
	}
	return nil
}

// validateLogLevel rejects a non-empty log level that does not parse. An empty
// level is valid and means "use the default" (info).
func validateLogLevel(level string) error {
	if level == "" {
		return nil
	}
	if _, err := logger.ParseLevel(level); err != nil {
		return err
	}
	return nil
}

// validateOptions validates ConfigOptions field invariants.
func validateOptions(opts *ConfigOptions) error {
	if err := validateLogLevel(opts.LogLevel); err != nil {
		return err
	}
	if err := validateConcurrency(opts.Concurrency); err != nil {
		return err
	}
	return validateTmpDir(opts.TmpDir)
}

// validateConcurrency enforces the [1, MaxConcurrency] range.
func validateConcurrency(concurrency int) error {
	if concurrency < 1 {
		return fmt.Errorf("concurrency must be >= 1, got %d: %w", concurrency, ErrInvalidConcurrency)
	}
	if concurrency > MaxConcurrency {
		return fmt.Errorf("concurrency must be <= %d, got %d: %w", MaxConcurrency, concurrency, ErrInvalidConcurrency)
	}
	return nil
}

// validateTmpDir asserts that, when set, TmpDir refers to an existing directory.
// An empty value is valid and means "use the OS default".
func validateTmpDir(path string) error {
	if path == "" {
		return nil
	}
	st, err := os.Stat(path) //nolint:gosec // user-provided tmp_dir is validated before use
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("tmp_dir: directory does not exist: %s: %w: %w", path, ErrInvalidTemporaryDirectory, err)
		}
		return fmt.Errorf("tmp_dir: failed to stat directory %s: %w: %w", path, ErrInvalidTemporaryDirectory, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("tmp_dir: path is not a directory: %s: %w", path, ErrInvalidTemporaryDirectory)
	}
	return nil
}
