// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
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

	result := r.OverlayCLI(cmd, base)
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

	result := r.OverlayCLI(cmd, base)

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

	result := r.OverlayCLI(cmd, base)

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

	result := r.OverlayCLI(cmd, base)
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

	result := r.OverlayCLI(cmd, base)
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

	result := r.OverlayCLI(cmd, base)
	assert.Equal(t, "debug", result.LogLevel, "CLI --log-level should override base")
}

func TestThreeLayerPrecedence(t *testing.T) {
	r := NewConfigResolver()

	tests := []struct {
		name          string
		hclOpts       *bundle.ConfigOptions // nil means no config.uds.hcl
		cliFlags      map[string]string     // flags to set via cmd.Flags().Set()
		wantLogLevel  string
		wantArch      string
		wantConc      int
		wantPlainHTTP bool
	}{
		{
			name:         "all defaults, no HCL, no CLI",
			hclOpts:      nil,
			cliFlags:     nil,
			wantLogLevel: "info",
			wantArch:     runtime.GOARCH,
			wantConc:     10,
		},
		{
			name:         "HCL overrides default",
			hclOpts:      &bundle.ConfigOptions{Architecture: "arm64"},
			cliFlags:     nil,
			wantLogLevel: "info",
			wantArch:     "arm64",
			wantConc:     10,
		},
		{
			name:         "CLI overrides default",
			hclOpts:      nil,
			cliFlags:     map[string]string{"architecture": "arm64"},
			wantLogLevel: "info",
			wantArch:     "arm64",
			wantConc:     10,
		},
		{
			name:         "CLI overrides HCL",
			hclOpts:      &bundle.ConfigOptions{Architecture: "arm64"},
			cliFlags:     map[string]string{"architecture": "s390x"},
			wantLogLevel: "info",
			wantArch:     "s390x",
			wantConc:     10,
		},
		{
			name:         "partial HCL (only concurrency)",
			hclOpts:      &bundle.ConfigOptions{Concurrency: 5},
			cliFlags:     nil,
			wantLogLevel: "info",
			wantArch:     runtime.GOARCH,
			wantConc:     5,
		},
		{
			name:          "HCL bool true, no CLI",
			hclOpts:       &bundle.ConfigOptions{PlainHTTP: true},
			cliFlags:      nil,
			wantLogLevel:  "info",
			wantArch:      runtime.GOARCH,
			wantConc:      10,
			wantPlainHTTP: true,
		},
		{
			name:         "HCL bool false (indistinguishable from unset)",
			hclOpts:      &bundle.ConfigOptions{PlainHTTP: false},
			cliFlags:     nil,
			wantLogLevel: "info",
			wantArch:     runtime.GOARCH,
			wantConc:     10,
		},
		{
			name:         "CLI sets concurrency to 1",
			hclOpts:      &bundle.ConfigOptions{Concurrency: 5},
			cliFlags:     map[string]string{"concurrency": "1"},
			wantLogLevel: "info",
			wantArch:     runtime.GOARCH,
			wantConc:     1,
		},
		{
			name:         "HCL overrides log_level",
			hclOpts:      &bundle.ConfigOptions{LogLevel: "debug"},
			cliFlags:     nil,
			wantLogLevel: "debug",
			wantArch:     runtime.GOARCH,
			wantConc:     10,
		},
		{
			name:         "CLI log-level overrides HCL log_level",
			hclOpts:      &bundle.ConfigOptions{LogLevel: "debug"},
			cliFlags:     map[string]string{"log-level": "error"},
			wantLogLevel: "error",
			wantArch:     runtime.GOARCH,
			wantConc:     10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: Start from defaults
			base := r.Defaults()

			// Step 2: Merge HCL options if provided
			if tt.hclOpts != nil {
				base = r.MergeHCL(base, tt.hclOpts)
			}

			// Step 3: Overlay CLI flags
			cmd := &cobra.Command{}
			registerTestFlags(cmd)
			for k, v := range tt.cliFlags {
				require.NoError(t, cmd.Flags().Set(k, v))
			}
			result := r.OverlayCLI(cmd, base)

			assert.Equal(t, tt.wantLogLevel, result.LogLevel)
			assert.Equal(t, tt.wantArch, result.Architecture)
			assert.Equal(t, tt.wantConc, result.Concurrency)
			assert.Equal(t, tt.wantPlainHTTP, result.PlainHTTP)
		})
	}
}

func TestResolve_DefaultsOnly(t *testing.T) {
	r := NewConfigResolver()
	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	cmd.Flags().String("config", "", "config path")

	resolved, configPath, err := r.Resolve(cmd)
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

	resolved, resolvedConfigPath, err := r.Resolve(cmd)
	require.NoError(t, err)

	require.NotNil(t, resolved.Global)
	assert.Equal(t, "info", resolved.Global.LogLevel)
	assert.Equal(t, "arm64", resolved.Options.Architecture)
	assert.Equal(t, 5, resolved.Options.Concurrency)
	assert.Equal(t, os.TempDir(), resolved.Options.TmpDir, "unset HCL fields should preserve defaults")
	assert.Equal(t, configPath, resolvedConfigPath)
	assert.Equal(t, "test-cluster", resolved.Variables["cluster_name"])
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

	resolved, _, err := r.Resolve(cmd)
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

	_, _, err := r.Resolve(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse config")
}

func TestResolve_NoConfigFlag(t *testing.T) {
	r := NewConfigResolver()
	// When --config flag doesn't exist on the command (e.g. not registered),
	// GetString returns "" and we skip HCL parsing.
	cmd := &cobra.Command{}
	registerTestFlags(cmd)
	// Deliberately not registering --config flag

	resolved, configPath, err := r.Resolve(cmd)
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

	resolved, _, err := r.Resolve(cmd)
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

	resolved, _, err := r.Resolve(cmd)
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

	resolved, _, err := r.Resolve(cmd)
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

	resolved, _, err := r.Resolve(cmd)
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

	_, _, err := r.Resolve(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}
