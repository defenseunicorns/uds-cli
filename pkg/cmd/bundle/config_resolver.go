// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	cmdconfig "github.com/defenseunicorns/uds-cli/pkg/cmd/config"
	"github.com/spf13/cobra"
)

// ConfigResolver encapsulates the four-layer config resolution logic:
// defaults → defaults.uds.hcl → config.uds.hcl → CLI flags.
type ConfigResolver struct{}

// NewConfigResolver returns a new ConfigResolver.
func NewConfigResolver() *ConfigResolver {
	return &ConfigResolver{}
}

// Defaults returns ConfigOptions with sensible defaults per ADR-0006.
func (r *ConfigResolver) Defaults() bundle.ConfigOptions {
	return bundle.ConfigOptions{
		LogLevel:     "info",
		Architecture: runtime.GOARCH,
		TmpDir:       os.TempDir(),
		Concurrency:  10,
	}
}

// MergeHCL merges HCL-parsed options into a defaults base.
// Only non-zero HCL values overwrite the base. This is necessary because
// gohcl decodes missing fields as Go zero values, so a partial options {}
// block would otherwise erase defaults for unset fields.
//
// Limitation: bool fields cannot distinguish "explicitly set to false" from
// "not set" in HCL. If a user writes `plain_http = false` in config.uds.hcl,
// it looks the same as omitting the field. This is acceptable because false
// is the default for all bool options.
func (r *ConfigResolver) MergeHCL(base bundle.ConfigOptions, hcl *bundle.ConfigOptions) bundle.ConfigOptions {
	if hcl == nil {
		return base
	}
	if hcl.LogLevel != "" {
		base.LogLevel = hcl.LogLevel
	}
	if hcl.Architecture != "" {
		base.Architecture = hcl.Architecture
	}
	if hcl.PlainHTTP {
		base.PlainHTTP = hcl.PlainHTTP
	}
	if hcl.SkipTLSVerify {
		base.SkipTLSVerify = hcl.SkipTLSVerify
	}
	if hcl.UDSCache != "" {
		base.UDSCache = hcl.UDSCache
	}
	if hcl.TmpDir != "" {
		base.TmpDir = hcl.TmpDir
	}
	if hcl.Concurrency != 0 {
		base.Concurrency = hcl.Concurrency
	}
	return base
}

// OverlayCLI overlays CLI flag values onto a base ConfigOptions.
// Only flags explicitly set by the user (cmd.Flags().Changed()) are applied.
// This ensures config.uds.hcl values are preserved unless the user overrides
// them on the command line.
func (r *ConfigResolver) OverlayCLI(cmd *cobra.Command, base bundle.ConfigOptions) bundle.ConfigOptions {
	if cmd.Flags().Changed("log-level") {
		base.LogLevel, _ = cmd.Flags().GetString("log-level")
	}
	if cmd.Flags().Changed("architecture") {
		base.Architecture, _ = cmd.Flags().GetString("architecture")
	}
	if cmd.Flags().Changed("plain-http") {
		base.PlainHTTP, _ = cmd.Flags().GetBool("plain-http")
	}
	if cmd.Flags().Changed("skip-tls-verify") {
		base.SkipTLSVerify, _ = cmd.Flags().GetBool("skip-tls-verify")
	}
	if cmd.Flags().Changed("tmp-dir") {
		base.TmpDir, _ = cmd.Flags().GetString("tmp-dir")
	}
	if cmd.Flags().Changed("concurrency") {
		base.Concurrency, _ = cmd.Flags().GetInt("concurrency")
	}
	return base
}

// Resolve resolves ConfigOptions through the four-layer precedence chain.
// bundlePath is the user-provided bundle path (directory or file); when non-empty,
// Resolve looks for defaults.uds.hcl in that directory. Pass "" to skip defaults.
// Returns the merged UDSBundleConfig and the config file path (empty if no --config flag).
//
//  1. Start from Defaults()
//  2. If defaults.uds.hcl exists in bundlePath, merge its variables (options are not supported)
//  3. If --config flag is set, parse config.uds.hcl and merge its options and variables
//  4. Overlay any explicitly-set CLI flags
//  5. Build GlobalOptions from merged log_level and the --prompt flag
func (r *ConfigResolver) Resolve(cmd *cobra.Command, bundlePath string) (*bundle.UDSBundleConfig, string, error) {
	base := r.Defaults()
	var variables bundle.Variables

	// Merge variables from defaults.uds.hcl, if exists
	defaults, err := r.loadBundleDefaultConfig(cmd.Context(), bundlePath)
	if err != nil {
		return nil, "", err
	}
	if defaults != nil {
		variables = defaults.Variables
	}

	// Merge config.uds.hcl if --config flag is set
	configPath, _ := cmd.Flags().GetString("config")
	if configPath != "" {
		cfg, err := bundle.NewHCLParser().ParseBundleConfig(cmd.Context(), configPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to parse config: %w", err)
		}
		if cfg.Options != nil {
			base = r.MergeHCL(base, cfg.Options)
		}
		variables = bundle.MergeVariables(variables, cfg.Variables)
	}

	merged := r.OverlayCLI(cmd, base)

	// Resolve global options (log level, prompt) via the shared config package.
	// This is command-group agnostic — future groups reuse the same function.
	global, err := cmdconfig.ResolveGlobalOptions(cmd, merged.LogLevel)
	if err != nil {
		return nil, "", err
	}

	return &bundle.UDSBundleConfig{
		Global:    global,
		Options:   &merged,
		Variables: variables,
	}, configPath, nil
}

// loadBundleDefaultConfig looks for defaults.uds.hcl in the bundle directory and parses it if present.
// Returns nil (not an error) when the file does not exist or bundlePath is not accessible. Invalid paths
// will be caught later by ValidateBundlePath.
func (r *ConfigResolver) loadBundleDefaultConfig(ctx context.Context, bundlePath string) (*bundle.UDSBundleConfig, error) {
	if bundlePath == "" {
		return nil, nil
	}

	dir := bundlePath
	info, err := os.Stat(bundlePath)
	if err != nil {
		// Log and skip defaults if bundlePath not found/accessible. ValidateBundlePath will report this error
		slog.Debug("bundle path not accessible, skipping defaults", "path", bundlePath, "error", err)
		return nil, nil
	}
	if !info.IsDir() {
		dir = filepath.Dir(bundlePath)
	}

	defaultsPath := filepath.Join(dir, bundle.DefaultBundleConfigFileName)
	if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to access defaults file: %w", err)
	}

	slog.Debug("loading bundle defaults", "path", defaultsPath)
	cfg, err := bundle.NewHCLParser().ParseBundleConfig(ctx, defaultsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", bundle.DefaultBundleConfigFileName, err)
	}

	// options block is not supported in defaults file
	if cfg.Options != nil {
		return nil, fmt.Errorf("%s does not support the options block", bundle.DefaultBundleConfigFileName)
	}
	return cfg, nil
}
