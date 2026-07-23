// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	cmdconfig "github.com/defenseunicorns/uds-cli/pkg/cmd/config"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// CLIFlags holds a snapshot of CLI flag values together with their Changed() bits.
// It exists so ConfigResolver.Resolve() does not depend on *cobra.Command, which
// lets command Run() methods stay cobra-free (per ADR-0011 and CLI-191).
type CLIFlags struct {
	ConfigPath           string // --config (always read; empty when unset)
	LogLevel             string
	LogLevelChanged      bool
	Architecture         string
	ArchitectureChanged  bool
	PlainHTTP            bool
	PlainHTTPChanged     bool
	SkipTLSVerify        bool
	SkipTLSVerifyChanged bool
	TmpDir               string
	TmpDirChanged        bool
	Concurrency          int
	ConcurrencyChanged   bool
	Prompt               bool
}

// SnapshotFlags reads every CLI flag the resolver needs from cmd, plus its Changed() bit.
// This is the only place that touches cobra.Command for config resolution in the post-CLI-191 pipeline.
func SnapshotFlags(cmd *cobra.Command) CLIFlags {
	var f CLIFlags
	f.ConfigPath, _ = cmd.Flags().GetString("config")
	f.LogLevel, _ = cmd.Flags().GetString("log-level")
	f.LogLevelChanged = cmd.Flags().Changed("log-level")
	f.Architecture, _ = cmd.Flags().GetString("architecture")
	f.ArchitectureChanged = cmd.Flags().Changed("architecture")
	f.PlainHTTP, _ = cmd.Flags().GetBool("plain-http")
	f.PlainHTTPChanged = cmd.Flags().Changed("plain-http")
	f.SkipTLSVerify, _ = cmd.Flags().GetBool("skip-tls-verify")
	f.SkipTLSVerifyChanged = cmd.Flags().Changed("skip-tls-verify")
	f.TmpDir, _ = cmd.Flags().GetString("tmp-dir")
	f.TmpDirChanged = cmd.Flags().Changed("tmp-dir")
	f.Concurrency, _ = cmd.Flags().GetInt("concurrency")
	f.ConcurrencyChanged = cmd.Flags().Changed("concurrency")
	f.Prompt, _ = cmd.Flags().GetBool("prompt")
	return f
}

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
// Only flags explicitly set by the user (Changed bits in flags) are applied.
// This ensures config.uds.hcl values are preserved unless the user overrides
// them on the command line.
func (r *ConfigResolver) OverlayCLI(flags CLIFlags, base bundle.ConfigOptions) bundle.ConfigOptions {
	if flags.LogLevelChanged {
		base.LogLevel = flags.LogLevel
	}
	if flags.ArchitectureChanged {
		base.Architecture = flags.Architecture
	}
	if flags.PlainHTTPChanged {
		base.PlainHTTP = flags.PlainHTTP
	}
	if flags.SkipTLSVerifyChanged {
		base.SkipTLSVerify = flags.SkipTLSVerify
	}
	if flags.TmpDirChanged {
		base.TmpDir = flags.TmpDir
	}
	if flags.ConcurrencyChanged {
		base.Concurrency = flags.Concurrency
	}
	return base
}

// Resolve resolves a full UDSBundleConfig through the four-layer precedence chain.
// bundlePath is the user-provided bundle path (directory or file); when non-empty,
// Resolve looks for defaults.uds.hcl in that directory. Pass "" to skip defaults.
// Returns the merged UDSBundleConfig and the config file path (empty if no --config flag).
//
//  1. Start from Defaults()
//  2. If defaults.uds.hcl exists in bundlePath, merge its variables (options are not supported)
//  3. If --config flag is set, parse config.uds.hcl and merge its options and variables
//  4. Overlay any explicitly-set CLI flags
//  5. Build GlobalOptions from merged log_level and the --prompt flag
func (r *ConfigResolver) Resolve(ctx context.Context, streams iostreams.IOStreams, flags CLIFlags, bundlePath string) (*bundle.UDSBundleConfig, string, error) {
	userCfg, err := r.parseUserConfig(ctx, streams, flags)
	if err != nil {
		return nil, "", err
	}

	options := r.resolveOptions(userCfg, flags)
	global, err := cmdconfig.ResolveGlobalOptions(flags.Prompt, options.LogLevel)
	if err != nil {
		return nil, "", err
	}

	// Merge variables: defaults.uds.hcl (base) then config.uds.hcl (overrides).
	defaults, err := r.loadBundleDefaults(ctx, streams, bundlePath)
	if err != nil {
		return nil, "", err
	}
	var variables bundle.Variables
	if defaults != nil {
		variables = defaults.Variables
	}
	if userCfg != nil {
		variables = bundle.MergeVariables(variables, userCfg.Variables)
	}

	return &bundle.UDSBundleConfig{
		Global:    global,
		Options:   &options,
		Variables: variables,
	}, flags.ConfigPath, nil
}

// parseUserConfig parses the config.uds.hcl referenced by --config, returning nil when unset.
func (r *ConfigResolver) parseUserConfig(ctx context.Context, streams iostreams.IOStreams, flags CLIFlags) (*bundle.UDSBundleConfig, error) {
	if flags.ConfigPath == "" {
		return nil, nil
	}
	cfg, err := bundle.NewHCLParser("", streams).ParseBundleConfig(ctx, flags.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return cfg, nil
}

// resolveOptions layers config.uds.hcl options and CLI flags onto Defaults().
func (r *ConfigResolver) resolveOptions(userCfg *bundle.UDSBundleConfig, flags CLIFlags) bundle.ConfigOptions {
	base := r.Defaults()
	if userCfg != nil && userCfg.Options != nil {
		base = r.MergeHCL(base, userCfg.Options)
	}
	return r.OverlayCLI(flags, base)
}

// loadBundleDefaults looks for defaults.uds.hcl in the bundle directory and parses it if present.
// Returns nil (not an error) when the file does not exist or bundlePath is not accessible.
func (r *ConfigResolver) loadBundleDefaults(ctx context.Context, streams iostreams.IOStreams, bundlePath string) (*bundle.UDSBundleConfig, error) {
	if bundlePath == "" {
		return nil, nil
	}

	dir := bundlePath
	info, err := os.Stat(bundlePath)
	if err != nil {
		// Log and skip defaults if bundlePath not found/accessible. ValidateBundlePath will report this error
		streams.Debug("bundle path not accessible, skipping defaults", "path", bundlePath, "error", err)
		return nil, nil
	}
	if !info.IsDir() {
		dir = filepath.Dir(bundlePath)
	}

	defaultsPath := filepath.Join(dir, bundle.BundleDefaultsFileName)
	if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to access defaults file: %w", err)
	}

	streams.Debug("loading bundle defaults", "path", defaultsPath)
	vars, err := bundle.ParseDefaults(ctx, defaultsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", bundle.BundleDefaultsFileName, err)
	}

	return &bundle.UDSBundleConfig{Variables: vars}, nil
}
