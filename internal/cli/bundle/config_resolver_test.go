// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaults(t *testing.T) {
	r := NewConfigResolver()
	opts := r.Defaults()

	require.Equal(t, "info", opts.LogLevel)
	require.Equal(t, runtime.GOARCH, opts.Architecture)
	require.False(t, opts.PlainHTTP)
	require.False(t, opts.SkipTLSVerify)
	require.Equal(t, 10, opts.Concurrency)
	require.Equal(t, os.TempDir(), opts.TmpDir)
	require.Empty(t, opts.UDSCache)
}

func TestMergeHCL_NilPointer(t *testing.T) {
	r := NewConfigResolver()
	base := bundle.ConfigOptions{
		LogLevel:     "info",
		Architecture: "amd64",
		TmpDir:       "/tmp",
		Concurrency:  10,
	}
	result := r.MergeHCL(base, nil)
	assert.Equal(t, base, result, "nil hcl should return base unchanged")
}

func TestMergeHCL(t *testing.T) {
	r := NewConfigResolver()

	tests := []struct {
		name     string
		base     bundle.ConfigOptions
		hcl      *bundle.ConfigOptions
		expected bundle.ConfigOptions
	}{
		{
			name: "nil-like HCL (all zero values) preserves base",
			base: bundle.ConfigOptions{
				LogLevel:     "info",
				Architecture: "amd64",
				TmpDir:       "/tmp",
				Concurrency:  10,
			},
			hcl: &bundle.ConfigOptions{},
			expected: bundle.ConfigOptions{
				LogLevel:     "info",
				Architecture: "amd64",
				TmpDir:       "/tmp",
				Concurrency:  10,
			},
		},
		{
			name: "HCL overrides architecture",
			base: bundle.ConfigOptions{
				LogLevel:     "info",
				Architecture: "amd64",
				TmpDir:       "/tmp",
				Concurrency:  10,
			},
			hcl: &bundle.ConfigOptions{
				Architecture: "arm64",
			},
			expected: bundle.ConfigOptions{
				LogLevel:     "info",
				Architecture: "arm64",
				TmpDir:       "/tmp",
				Concurrency:  10,
			},
		},
		{
			name: "HCL overrides concurrency only",
			base: bundle.ConfigOptions{
				LogLevel:     "info",
				Architecture: "amd64",
				TmpDir:       "/tmp",
				Concurrency:  10,
			},
			hcl: &bundle.ConfigOptions{
				Concurrency: 5,
			},
			expected: bundle.ConfigOptions{
				LogLevel:     "info",
				Architecture: "amd64",
				TmpDir:       "/tmp",
				Concurrency:  5,
			},
		},
		{
			name: "HCL sets PlainHTTP true",
			base: bundle.ConfigOptions{
				LogLevel:     "info",
				Architecture: "amd64",
				Concurrency:  10,
			},
			hcl: &bundle.ConfigOptions{
				PlainHTTP: true,
			},
			expected: bundle.ConfigOptions{
				LogLevel:     "info",
				Architecture: "amd64",
				Concurrency:  10,
				PlainHTTP:    true,
			},
		},
		{
			name: "HCL sets all fields",
			base: bundle.ConfigOptions{
				LogLevel:     "info",
				Architecture: "amd64",
				TmpDir:       "/tmp",
				Concurrency:  10,
			},
			hcl: &bundle.ConfigOptions{
				LogLevel:      "debug",
				Architecture:  "arm64",
				PlainHTTP:     true,
				SkipTLSVerify: true,
				UDSCache:      "/cache",
				TmpDir:        "/custom-tmp",
				Concurrency:   20,
			},
			expected: bundle.ConfigOptions{
				LogLevel:      "debug",
				Architecture:  "arm64",
				PlainHTTP:     true,
				SkipTLSVerify: true,
				UDSCache:      "/cache",
				TmpDir:        "/custom-tmp",
				Concurrency:   20,
			},
		},
		{
			name: "HCL bool false is indistinguishable from unset (preserves base)",
			base: bundle.ConfigOptions{
				PlainHTTP: false,
			},
			hcl: &bundle.ConfigOptions{
				PlainHTTP: false, // looks same as unset
			},
			expected: bundle.ConfigOptions{
				PlainHTTP: false,
			},
		},
		{
			name: "HCL overrides log_level",
			base: bundle.ConfigOptions{
				LogLevel:     "info",
				Architecture: "amd64",
				Concurrency:  10,
			},
			hcl: &bundle.ConfigOptions{
				LogLevel: "debug",
			},
			expected: bundle.ConfigOptions{
				LogLevel:     "debug",
				Architecture: "amd64",
				Concurrency:  10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.MergeHCL(tt.base, tt.hcl)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// registerTestFlags mirrors the persistent flags from NewBundleCommand and root command.
func registerTestFlags(cmd *cobra.Command) {
	r := NewConfigResolver()
	defaults := r.Defaults()
	cmd.Flags().StringP("log-level", "l", "info", "log level")
	cmd.Flags().Bool("prompt", false, "enable interactive confirmation prompts")
	cmd.Flags().StringP("architecture", "a", defaults.Architecture, "target architecture")
	cmd.Flags().Bool("plain-http", defaults.PlainHTTP, "use plain HTTP")
	cmd.Flags().Bool("skip-tls-verify", defaults.SkipTLSVerify, "skip TLS verification")
	cmd.Flags().String("tmp-dir", defaults.TmpDir, "temp directory")
	cmd.Flags().Int("concurrency", defaults.Concurrency, "concurrency")
}

func TestOverlayCLI_NoFlagsChanged(t *testing.T) {
	r := NewConfigResolver()
	cmd := &cobra.Command{}
	registerTestFlags(cmd)

	base := bundle.ConfigOptions{
		LogLevel:     "debug",
		Architecture: "arm64",
		Concurrency:  5,
		TmpDir:       "/custom",
	}

	result := r.OverlayCLI(SnapshotFlags(cmd), base)
	assert.Equal(t, base, result, "no flags changed; base should be returned unchanged")
}

func TestOverlayCLI_AllFlagsChanged(t *testing.T) {
	r := NewConfigResolver()
	cmd := &cobra.Command{}
	registerTestFlags(cmd)

	require.NoError(t, cmd.Flags().Set("log-level", "error"))
	require.NoError(t, cmd.Flags().Set("architecture", "s390x"))
	require.NoError(t, cmd.Flags().Set("plain-http", "true"))
	require.NoError(t, cmd.Flags().Set("skip-tls-verify", "true"))
	require.NoError(t, cmd.Flags().Set("tmp-dir", "/cli-tmp"))
	require.NoError(t, cmd.Flags().Set("concurrency", "42"))

	base := bundle.ConfigOptions{
		LogLevel:     "info",
		Architecture: "arm64",
		Concurrency:  5,
		TmpDir:       "/hcl-tmp",
	}

	result := r.OverlayCLI(SnapshotFlags(cmd), base)

	assert.Equal(t, "error", result.LogLevel)
	assert.Equal(t, "s390x", result.Architecture)
	assert.True(t, result.PlainHTTP)
	assert.True(t, result.SkipTLSVerify)
	assert.Equal(t, "/cli-tmp", result.TmpDir)
	assert.Equal(t, 42, result.Concurrency)
}

func TestOverlayCLI_PartialFlags(t *testing.T) {
	r := NewConfigResolver()
	cmd := &cobra.Command{}
	registerTestFlags(cmd)

	require.NoError(t, cmd.Flags().Set("architecture", "arm64"))

	base := bundle.ConfigOptions{
		LogLevel:     "info",
		Architecture: "amd64",
		Concurrency:  5,
		TmpDir:       "/hcl-tmp",
		PlainHTTP:    true,
	}

	result := r.OverlayCLI(SnapshotFlags(cmd), base)

	assert.Equal(t, "info", result.LogLevel, "unchanged flag should preserve base")
	assert.Equal(t, "arm64", result.Architecture, "CLI flag should override")
	assert.Equal(t, 5, result.Concurrency, "unchanged flag should preserve base")
	assert.Equal(t, "/hcl-tmp", result.TmpDir, "unchanged flag should preserve base")
	assert.True(t, result.PlainHTTP, "unchanged flag should preserve base")
}

func TestOverlayCLI_CLIOverridesHCL(t *testing.T) {
	r := NewConfigResolver()
	cmd := &cobra.Command{}
	registerTestFlags(cmd)

	require.NoError(t, cmd.Flags().Set("architecture", "s390x"))

	// Simulate HCL having set architecture to arm64
	base := bundle.ConfigOptions{
		Architecture: "arm64",
		Concurrency:  10,
	}

	result := r.OverlayCLI(SnapshotFlags(cmd), base)
	assert.Equal(t, "s390x", result.Architecture, "CLI should override HCL")
}

func TestOverlayCLI_CLISetsZeroishValue(t *testing.T) {
	r := NewConfigResolver()
	cmd := &cobra.Command{}
	registerTestFlags(cmd)

	require.NoError(t, cmd.Flags().Set("concurrency", "1"))

	base := bundle.ConfigOptions{
		Architecture: runtime.GOARCH,
		Concurrency:  5,
	}

	result := r.OverlayCLI(SnapshotFlags(cmd), base)
	assert.Equal(t, 1, result.Concurrency, "CLI should set concurrency to 1")
}

func TestOverlayCLI_LogLevelChanged(t *testing.T) {
	r := NewConfigResolver()
	cmd := &cobra.Command{}
	registerTestFlags(cmd)

	require.NoError(t, cmd.Flags().Set("log-level", "debug"))

	base := bundle.ConfigOptions{
		LogLevel:     "info",
		Architecture: runtime.GOARCH,
		Concurrency:  10,
	}

	result := r.OverlayCLI(SnapshotFlags(cmd), base)
	assert.Equal(t, "debug", result.LogLevel, "CLI --log-level should override base")
}

func TestMultiLayerPrecedence(t *testing.T) {
	r := NewConfigResolver()
	builtinLogLevel := "info"
	builtinArch := runtime.GOARCH
	builtinConcurrency := 10
	builtinPlainHTTP := false

	tests := []struct {
		name            string
		configFileOpts  *bundle.ConfigOptions // config.uds.hcl options, nil means no specified
		cliFlags        map[string]string     // layer 4: CLI flags
		defaultFileVars bundle.Variables      // defaults.uds.hcl (variables only), nil means no file
		configFileVars  bundle.Variables      // config.uds.hcl variables, nil means not specified
		wantLogLevel    string
		wantArch        string
		wantConc        int
		wantPlainHTTP   bool
		wantVars        bundle.Variables
	}{
		// -- Option Overrides
		{
			name:            "no defaults file, no config file, no CLI options uses built-in default options and no variables",
			configFileOpts:  nil,
			cliFlags:        nil,
			defaultFileVars: nil,
			configFileVars:  nil,
			wantLogLevel:    builtinLogLevel,
			wantArch:        builtinArch,
			wantConc:        builtinConcurrency,
			wantPlainHTTP:   builtinPlainHTTP,
			wantVars:        nil,
		},
		{
			name:            "config file options overrides built-in default options",
			configFileOpts:  &bundle.ConfigOptions{LogLevel: "debug", Architecture: "ia64", Concurrency: 1, PlainHTTP: true},
			cliFlags:        nil,
			defaultFileVars: nil,
			configFileVars:  nil,
			wantLogLevel:    "debug",
			wantArch:        "ia64",
			wantConc:        1,
			wantPlainHTTP:   true,
			wantVars:        nil,
		},
		{
			name:            "config file options partial overrides built-in default options",
			configFileOpts:  &bundle.ConfigOptions{LogLevel: "debug", Concurrency: 1},
			cliFlags:        nil,
			defaultFileVars: nil,
			configFileVars:  nil,
			wantLogLevel:    "debug",
			wantArch:        builtinArch,
			wantConc:        1,
			wantPlainHTTP:   builtinPlainHTTP,
			wantVars:        nil,
		},
		{
			name:            "CLI options overrides built-in default options",
			configFileOpts:  nil,
			cliFlags:        map[string]string{"log-level": "warning", "architecture": "s390x", "concurrency": "2", "plain-http": "true"},
			defaultFileVars: nil,
			configFileVars:  nil,
			wantLogLevel:    "warning",
			wantArch:        "s390x",
			wantConc:        2,
			wantPlainHTTP:   true,
			wantVars:        nil,
		},
		{
			name:            "CLI options partial overrides built-in default options",
			configFileOpts:  nil,
			cliFlags:        map[string]string{"log-level": "warning", "architecture": "s390x"},
			defaultFileVars: nil,
			configFileVars:  nil,
			wantLogLevel:    "warning",
			wantArch:        "s390x",
			wantConc:        builtinConcurrency,
			wantPlainHTTP:   builtinPlainHTTP,
			wantVars:        nil,
		},
		{
			name:            "CLI options overrides config file options",
			configFileOpts:  &bundle.ConfigOptions{LogLevel: "debug", Architecture: "ia64", Concurrency: 1, PlainHTTP: true},
			cliFlags:        map[string]string{"log-level": "warning", "architecture": "s390x", "concurrency": "2", "plain-http": "false"},
			defaultFileVars: nil,
			configFileVars:  nil,
			wantLogLevel:    "warning",
			wantArch:        "s390x",
			wantConc:        2,
			wantPlainHTTP:   false,
			wantVars:        nil,
		},
		{
			name:            "CLI options partial overrides config file options",
			configFileOpts:  &bundle.ConfigOptions{LogLevel: "debug", Architecture: "ia64", Concurrency: 1, PlainHTTP: true},
			cliFlags:        map[string]string{"architecture": "s390x", "concurrency": "2"},
			defaultFileVars: nil,
			configFileVars:  nil,
			wantLogLevel:    "debug",
			wantArch:        "s390x",
			wantConc:        2,
			wantPlainHTTP:   true,
			wantVars:        nil,
		},
		// -- Variable overrides
		{
			name:            "defaults file vars applied with no config file vars",
			configFileOpts:  nil,
			cliFlags:        nil,
			defaultFileVars: bundle.Variables{"a": "a-default-value", "b": bundle.Variables{"c": true, "d": "d-default-value"}},
			configFileVars:  nil,
			wantLogLevel:    builtinLogLevel,
			wantArch:        builtinArch,
			wantConc:        builtinConcurrency,
			wantPlainHTTP:   builtinPlainHTTP,
			wantVars:        bundle.Variables{"a": "a-default-value", "b": bundle.Variables{"c": true, "d": "d-default-value"}},
		},
		{
			name:            "config file vars applied with no default file vars",
			configFileOpts:  nil,
			cliFlags:        nil,
			defaultFileVars: nil,
			configFileVars:  bundle.Variables{"a": "a-config-value", "b": bundle.Variables{"c": false, "d": "d-config-value"}},
			wantLogLevel:    builtinLogLevel,
			wantArch:        builtinArch,
			wantConc:        builtinConcurrency,
			wantPlainHTTP:   builtinPlainHTTP,
			wantVars:        bundle.Variables{"a": "a-config-value", "b": bundle.Variables{"c": false, "d": "d-config-value"}},
		},
		{
			name:            "config file variables overrides default file variables",
			configFileOpts:  nil,
			cliFlags:        nil,
			defaultFileVars: bundle.Variables{"a": "a-default-value", "b": bundle.Variables{"c": true, "d": "d-default-value"}},
			configFileVars:  bundle.Variables{"a": "a-config-value", "b": bundle.Variables{"c": false, "d": "d-config-value"}},
			wantLogLevel:    builtinLogLevel,
			wantArch:        builtinArch,
			wantConc:        builtinConcurrency,
			wantPlainHTTP:   builtinPlainHTTP,
			wantVars:        bundle.Variables{"a": "a-config-value", "b": bundle.Variables{"c": false, "d": "d-config-value"}},
		},
		{
			name:            "config file vars partial override defaults file vars",
			configFileOpts:  nil,
			cliFlags:        nil,
			defaultFileVars: bundle.Variables{"a": "a-default-value", "b": bundle.Variables{"c": true, "d": "d-default-value"}},
			configFileVars:  bundle.Variables{"a": "a-config-value"},
			wantLogLevel:    builtinLogLevel,
			wantArch:        builtinArch,
			wantConc:        builtinConcurrency,
			wantPlainHTTP:   builtinPlainHTTP,
			wantVars:        bundle.Variables{"a": "a-config-value", "b": bundle.Variables{"c": true, "d": "d-default-value"}},
		},
		{
			name:            "config file vars partial override nested map defaults file vars",
			configFileOpts:  nil,
			cliFlags:        nil,
			defaultFileVars: bundle.Variables{"a": "a-default-value", "b": bundle.Variables{"c": true, "d": "d-default-value"}},
			configFileVars:  bundle.Variables{"b": bundle.Variables{"d": "d-config-value"}},
			wantLogLevel:    builtinLogLevel,
			wantArch:        builtinArch,
			wantConc:        builtinConcurrency,
			wantPlainHTTP:   builtinPlainHTTP,
			wantVars:        bundle.Variables{"a": "a-default-value", "b": bundle.Variables{"c": true, "d": "d-config-value"}},
		},
		{
			name:            "default and config file vars merged if no conflict",
			configFileOpts:  nil,
			cliFlags:        nil,
			defaultFileVars: bundle.Variables{"a": "a-default-value", "b": bundle.Variables{"c": true, "d": "d-default-value"}},
			configFileVars:  bundle.Variables{"e": "e-config-value", "f": bundle.Variables{"g": false, "h": "h-config-value"}},
			wantLogLevel:    builtinLogLevel,
			wantArch:        builtinArch,
			wantConc:        builtinConcurrency,
			wantPlainHTTP:   builtinPlainHTTP,
			wantVars: bundle.Variables{
				"a": "a-default-value",
				"b": bundle.Variables{"c": true, "d": "d-default-value"},
				"e": "e-config-value",
				"f": bundle.Variables{"g": false, "h": "h-config-value"},
			},
		},
		// -- Config file has both options and variables
		{
			name:            "config file options and variables are applied independently",
			configFileOpts:  &bundle.ConfigOptions{LogLevel: "debug", Architecture: "ia64"},
			cliFlags:        nil,
			defaultFileVars: bundle.Variables{"a": "a-default-value"},
			configFileVars:  bundle.Variables{"a": "a-config-value", "b": "b-config-value"},
			wantLogLevel:    "debug",
			wantArch:        "ia64",
			wantConc:        builtinConcurrency,
			wantPlainHTTP:   builtinPlainHTTP,
			wantVars:        bundle.Variables{"a": "a-config-value", "b": "b-config-value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Layer 1: Go defaults
			base := r.Defaults()

			// Layer 2: defaults.uds.hcl variables (options not supported)
			vars := bundle.MergeVariables(nil, tt.defaultFileVars)

			// Layer 3: config.uds.hcl options and variables
			if tt.configFileOpts != nil {
				base = r.MergeHCL(base, tt.configFileOpts)
			}
			vars = bundle.MergeVariables(vars, tt.configFileVars)

			// Layer 4: CLI flags (options only; variables are not settable via CLI)
			cmd := &cobra.Command{}
			registerTestFlags(cmd)
			for k, v := range tt.cliFlags {
				require.NoError(t, cmd.Flags().Set(k, v))
			}
			result := r.OverlayCLI(SnapshotFlags(cmd), base)

			assert.Equal(t, tt.wantLogLevel, result.LogLevel)
			assert.Equal(t, tt.wantArch, result.Architecture)
			assert.Equal(t, tt.wantConc, result.Concurrency)
			assert.Equal(t, tt.wantPlainHTTP, result.PlainHTTP)
			assert.Equal(t, tt.wantVars, vars)
		})
	}
}

func TestResolve_DefaultsOnly(t *testing.T) {
	r := NewConfigResolver()
	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")

	resolved, configPath, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), "")
	require.NoError(t, err)

	require.NotNil(t, resolved.Global)
	assert.Equal(t, "info", resolved.Global.LogLevel)
	assert.False(t, resolved.Global.Prompt)
	assert.Equal(t, "info", resolved.Options.LogLevel)
	assert.Equal(t, runtime.GOARCH, resolved.Options.Architecture)
	assert.Equal(t, os.TempDir(), resolved.Options.TmpDir)
	assert.Equal(t, 10, resolved.Options.Concurrency)
	assert.False(t, resolved.Options.PlainHTTP)
	assert.Nil(t, resolved.Variables)
	assert.Empty(t, configPath)
}

func TestResolve_WithHCLConfig(t *testing.T) {
	r := NewConfigResolver()
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.uds.hcl")
	require.NoError(t, os.WriteFile(configPath, []byte(`
options {
  architecture = "arm64"
  concurrency  = 5
}

variables = {
  cluster_name = "test-cluster"
}
`), 0o644))

	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")
	require.NoError(t, cmd.Flags().Set("config", configPath))

	resolved, resolvedConfigPath, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), "")
	require.NoError(t, err)

	require.NotNil(t, resolved.Global)
	assert.Equal(t, "info", resolved.Global.LogLevel)
	assert.Equal(t, "arm64", resolved.Options.Architecture)
	assert.Equal(t, 5, resolved.Options.Concurrency)
	assert.Equal(t, os.TempDir(), resolved.Options.TmpDir, "unset HCL fields should preserve defaults")
	assert.Equal(t, configPath, resolvedConfigPath)
	assert.Equal(t, "test-cluster", resolved.Variables["cluster_name"])
}

func TestResolveBaseAndApplyBundleDefaults(t *testing.T) {
	r := NewConfigResolver()
	bundleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, bundle.BundleDefaultsFileName), []byte(`
variables = {
  from_defaults = "default"
  overridden    = "default"
}
`), 0o600))
	configPath := filepath.Join(t.TempDir(), "config.uds.hcl")
	require.NoError(t, os.WriteFile(configPath, []byte(`
options {
  architecture = "arm64"
}
variables = {
  overridden  = "config"
  from_config = "config"
}
signature_verification {
  public_key = "trusted-public-key"
}
`), 0o600))

	flags := CLIFlags{
		ConfigPath:         configPath,
		PlainHTTP:          true,
		PlainHTTPChanged:   true,
		Concurrency:        3,
		ConcurrencyChanged: true,
		TmpDir:             bundleDir,
		TmpDirChanged:      true,
		Prompt:             true,
	}
	base, gotConfigPath, err := r.resolveBase(t.Context(), iostreams.IOStreams{}, flags)
	require.NoError(t, err)
	assert.Equal(t, configPath, gotConfigPath)
	assert.Equal(t, "arm64", base.Options.Architecture)
	assert.True(t, base.Options.PlainHTTP)
	assert.Equal(t, 3, base.Options.Concurrency)
	assert.Equal(t, bundleDir, base.Options.TmpDir)
	assert.True(t, base.Global.Prompt)
	assert.Equal(t, bundle.Variables{"overridden": "config", "from_config": "config"}, base.Variables)
	require.NotNil(t, base.SignatureVerification)
	assert.Equal(t, "trusted-public-key", base.SignatureVerification.PublicKey)

	resolved, err := r.applyBundleDefaults(t.Context(), iostreams.IOStreams{}, base, bundleDir)
	require.NoError(t, err)
	assert.Equal(t, "default", resolved.Variables["from_defaults"])
	assert.Equal(t, "config", resolved.Variables["overridden"])
	assert.Equal(t, "config", resolved.Variables["from_config"])
	assert.Equal(t, base.Options, resolved.Options)
	assert.Equal(t, base.Global, resolved.Global)
	assert.NotSame(t, base.Options, resolved.Options)
	assert.NotSame(t, base.Global, resolved.Global)
	resolved.Options.Concurrency = 99
	resolved.Global.LogLevel = "debug"
	assert.Equal(t, 3, base.Options.Concurrency)
	assert.Equal(t, "info", base.Global.LogLevel)
	assert.Equal(t, bundle.Variables{"overridden": "config", "from_config": "config"}, base.Variables, "base config must not be mutated")
}

func TestResolve_CLIOverridesHCL(t *testing.T) {
	r := NewConfigResolver()
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.uds.hcl")
	require.NoError(t, os.WriteFile(configPath, []byte(`
options {
  architecture = "arm64"
  concurrency  = 5
}
`), 0o644))

	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")
	require.NoError(t, cmd.Flags().Set("config", configPath))
	require.NoError(t, cmd.Flags().Set("architecture", "s390x"))

	resolved, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), "")
	require.NoError(t, err)

	assert.Equal(t, "s390x", resolved.Options.Architecture, "CLI should override HCL")
	assert.Equal(t, 5, resolved.Options.Concurrency, "non-overridden HCL value should persist")
}

func TestResolve_InvalidConfigPath(t *testing.T) {
	r := NewConfigResolver()
	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")
	require.NoError(t, cmd.Flags().Set("config", "/nonexistent/config.uds.hcl"))

	_, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), "")
	require.ErrorContains(t, err, "failed to parse config")
}

func TestResolve_NoConfigFlag(t *testing.T) {
	r := NewConfigResolver()
	// When --config flag doesn't exist on the command (e.g. not registered),
	// GetString returns "" and we skip HCL parsing.
	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	// Deliberately not registering --config flag

	resolved, configPath, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), "")
	require.NoError(t, err)

	require.NotNil(t, resolved.Global)
	assert.Equal(t, "info", resolved.Global.LogLevel)
	assert.Equal(t, runtime.GOARCH, resolved.Options.Architecture)
	assert.Empty(t, configPath)
	assert.Nil(t, resolved.Variables)
}

func TestResolve_LogLevelCLIOverride(t *testing.T) {
	r := NewConfigResolver()
	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")
	require.NoError(t, cmd.Flags().Set("log-level", "debug"))

	resolved, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), "")
	require.NoError(t, err)

	require.NotNil(t, resolved.Global)
	assert.Equal(t, "debug", resolved.Global.LogLevel, "CLI --log-level should override default")
	assert.Equal(t, "debug", resolved.Options.LogLevel, "Options.LogLevel should match Global.LogLevel")
}

func TestResolve_HCLLogLevelOverride(t *testing.T) {
	r := NewConfigResolver()
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.uds.hcl")
	require.NoError(t, os.WriteFile(configPath, []byte(`
options {
  log_level = "warn"
}
`), 0o644))

	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")
	require.NoError(t, cmd.Flags().Set("config", configPath))

	resolved, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), "")
	require.NoError(t, err)

	require.NotNil(t, resolved.Global)
	assert.Equal(t, "warn", resolved.Global.LogLevel, "HCL log_level should override default")
	assert.Equal(t, "warn", resolved.Options.LogLevel)
}

func TestResolve_CLILogLevelOverridesHCL(t *testing.T) {
	r := NewConfigResolver()
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.uds.hcl")
	require.NoError(t, os.WriteFile(configPath, []byte(`
options {
  log_level = "warn"
}
`), 0o644))

	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")
	require.NoError(t, cmd.Flags().Set("config", configPath))
	require.NoError(t, cmd.Flags().Set("log-level", "error"))

	resolved, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), "")
	require.NoError(t, err)

	require.NotNil(t, resolved.Global)
	assert.Equal(t, "error", resolved.Global.LogLevel, "CLI --log-level should override HCL log_level")
	assert.Equal(t, "error", resolved.Options.LogLevel)
}

func TestResolve_PromptFromFlag(t *testing.T) {
	r := NewConfigResolver()
	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")
	require.NoError(t, cmd.Flags().Set("prompt", "true"))

	resolved, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), "")
	require.NoError(t, err)

	require.NotNil(t, resolved.Global)
	assert.True(t, resolved.Global.Prompt, "--prompt flag should be reflected in GlobalOptions")
}

func TestResolve_InvalidHCLLogLevel(t *testing.T) {
	r := NewConfigResolver()
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.uds.hcl")
	require.NoError(t, os.WriteFile(configPath, []byte(`
options {
  log_level = "invalid-level"
}
`), 0o644))

	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")
	require.NoError(t, cmd.Flags().Set("config", configPath))

	_, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), "")
	require.ErrorContains(t, err, "invalid log level")
}

// --- defaults.uds.hcl tests ---

func TestResolve_DefaultsFile_VariablesApplied(t *testing.T) {
	r := NewConfigResolver()
	bundleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, bundle.BundleDefaultsFileName), []byte(`
variables = {
  domain = "default.dev"
  feature = {
    auth = true
  }
}
`), 0o644))

	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")

	resolved, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), bundleDir)
	require.NoError(t, err)

	assert.Equal(t, "default.dev", resolved.Variables["domain"])
	feature, ok := resolved.Variables["feature"].(bundle.Variables)
	require.True(t, ok)
	assert.Equal(t, true, feature["auth"])
}

func TestResolve_NoDefaultsFile_Skipped(t *testing.T) {
	r := NewConfigResolver()
	bundleDir := t.TempDir() // no defaults.uds.hcl

	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")

	resolved, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), bundleDir)
	require.NoError(t, err)

	assert.Equal(t, runtime.GOARCH, resolved.Options.Architecture, "should use Go defaults when no defaults file")
	assert.Nil(t, resolved.Variables)
}

func TestResolve_InvalidBundleDir_Skipped(t *testing.T) {
	r := NewConfigResolver()

	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")

	resolved, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), "/nonexistent/path")
	require.NoError(t, err, "invalid bundleDir should be skipped; ValidateBundlePath catches this later")
	assert.Equal(t, runtime.GOARCH, resolved.Options.Architecture)
}

func TestResolve_DefaultsFile_OptionsBlockNotAllowed(t *testing.T) {
	r := NewConfigResolver()
	bundleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, bundle.BundleDefaultsFileName), []byte(`
options {
  architecture = "amd64"
}
`), 0o644))

	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")

	_, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), bundleDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), bundle.BundleDefaultsFileName)
	assert.Contains(t, err.Error(), "block")
}

func TestResolve_DefaultsFile_InvalidHCL(t *testing.T) {
	r := NewConfigResolver()
	bundleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, bundle.BundleDefaultsFileName), []byte(`
this is not valid HCL {{{
`), 0o644))

	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")

	_, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), bundleDir)
	require.ErrorContains(t, err, bundle.BundleDefaultsFileName)
}

func TestResolve_DefaultsFile_BundleDirIsFilePath(t *testing.T) {
	r := NewConfigResolver()
	bundleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, bundle.BundleDefaultsFileName), []byte(`
variables = {
  a = "a-default-value"
}
`), 0o644))
	// Simulate passing a file path (e.g. /path/to/bundle.uds.hcl) instead of directory
	bundleFilePath := filepath.Join(bundleDir, bundle.BundleFileName)
	require.NoError(t, os.WriteFile(bundleFilePath, []byte(""), 0o644))

	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")

	resolved, _, err := r.Resolve(t.Context(), iostreams.IOStreams{}, SnapshotFlags(cmd), bundleFilePath)
	require.NoError(t, err)

	assert.Equal(t, "a-default-value", resolved.Variables["a"], "should find defaults.uds.hcl in parent dir of file path")
}

// --- SnapshotFlags tests ---

func TestSnapshotFlags_NoFlagsChanged(t *testing.T) {
	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")

	f := SnapshotFlags(cmd)

	assert.False(t, f.LogLevelChanged)
	assert.False(t, f.ArchitectureChanged)
	assert.False(t, f.PlainHTTPChanged)
	assert.False(t, f.SkipTLSVerifyChanged)
	assert.False(t, f.TmpDirChanged)
	assert.False(t, f.ConcurrencyChanged)
	assert.Empty(t, f.ConfigPath)
}

func TestSnapshotFlags_AllFlagsChanged(t *testing.T) {
	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")

	require.NoError(t, cmd.Flags().Set("log-level", "debug"))
	require.NoError(t, cmd.Flags().Set("architecture", "arm64"))
	require.NoError(t, cmd.Flags().Set("plain-http", "true"))
	require.NoError(t, cmd.Flags().Set("skip-tls-verify", "true"))
	require.NoError(t, cmd.Flags().Set("tmp-dir", "/custom"))
	require.NoError(t, cmd.Flags().Set("concurrency", "5"))
	require.NoError(t, cmd.Flags().Set("prompt", "true"))
	require.NoError(t, cmd.Flags().Set("config", "/some/config.uds.hcl"))

	f := SnapshotFlags(cmd)

	assert.True(t, f.LogLevelChanged)
	assert.Equal(t, "debug", f.LogLevel)
	assert.True(t, f.ArchitectureChanged)
	assert.Equal(t, "arm64", f.Architecture)
	assert.True(t, f.PlainHTTPChanged)
	assert.True(t, f.PlainHTTP)
	assert.True(t, f.SkipTLSVerifyChanged)
	assert.True(t, f.SkipTLSVerify)
	assert.True(t, f.TmpDirChanged)
	assert.Equal(t, "/custom", f.TmpDir)
	assert.True(t, f.ConcurrencyChanged)
	assert.Equal(t, 5, f.Concurrency)
	assert.True(t, f.Prompt)
	assert.Equal(t, "/some/config.uds.hcl", f.ConfigPath)
}

func TestSnapshotFlags_MissingFlagsNoPanic(t *testing.T) {
	// SnapshotFlags on a command that has none of the expected flags must not panic.
	cmd := &cobra.Command{}
	// No flags registered at all

	require.NotPanics(t, func() {
		f := SnapshotFlags(cmd)
		// All Changed bits must be false; values are zero
		assert.False(t, f.LogLevelChanged)
		assert.False(t, f.ConcurrencyChanged)
	})
}
