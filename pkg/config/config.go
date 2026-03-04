// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package config provides configuration types and defaults for UDS CLI commands.
package config

import (
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

// BundleConfig holds configuration for bundle component operations.
// See ADR-0006 for the full rationale behind each option.
type BundleConfig struct {
	// Architecture is the target CPU architecture (e.g. "amd64", "arm64").
	Architecture string

	// PlainHTTP allows communication with registries over plain HTTP.
	PlainHTTP bool

	// SkipTLSVerify skips TLS certificate verification for registries.
	SkipTLSVerify bool

	// TmpDir is the base directory for temporary working files.
	TmpDir string

	// Concurrency controls the degree of parallelism for concurrent operations.
	Concurrency int
}

// DefaultBundleConfig returns a BundleConfig populated with sensible defaults.
func DefaultBundleConfig() BundleConfig {
	return BundleConfig{
		Architecture: runtime.GOARCH,
		TmpDir:       os.TempDir(),
		Concurrency:  10,
	}
}

// BuildBundleConfig reads CLI flag values into a BundleConfig.
// It starts from DefaultBundleConfig() and only overwrites fields when the
// flag lookup succeeds, preventing zero-value fallthrough if flags are missing.
func BuildBundleConfig(cmd *cobra.Command) BundleConfig {
	cfg := DefaultBundleConfig()
	if v, err := cmd.Flags().GetString("architecture"); err == nil {
		cfg.Architecture = v
	}
	if v, err := cmd.Flags().GetBool("plain-http"); err == nil {
		cfg.PlainHTTP = v
	}
	if v, err := cmd.Flags().GetBool("skip-tls-verify"); err == nil {
		cfg.SkipTLSVerify = v
	}
	if v, err := cmd.Flags().GetString("tmp-dir"); err == nil {
		cfg.TmpDir = v
	}
	if v, err := cmd.Flags().GetInt("concurrency"); err == nil {
		cfg.Concurrency = v
	}
	return cfg
}
