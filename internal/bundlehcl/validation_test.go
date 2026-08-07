// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundlehcl

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validBaseConfig returns a fully-valid UDSBundleConfig used as the starting
// point for table-driven validation tests.
func validBaseConfig() *UDSBundleConfig {
	return &UDSBundleConfig{
		Global:  &GlobalOptions{LogLevel: "info"},
		Options: &ConfigOptions{Concurrency: 10},
	}
}

func TestValidateConfig_Structure(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *UDSBundleConfig
		wantErr string
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: "config is required",
		},
		{
			name: "nil Global",
			cfg: &UDSBundleConfig{
				Options: &ConfigOptions{Concurrency: 10},
			},
			wantErr: "config.Global is required",
		},
		{
			name: "nil Options",
			cfg: &UDSBundleConfig{
				Global: &GlobalOptions{},
			},
			wantErr: "config.Options is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	require.NoError(t, ValidateConfig(validBaseConfig()))
}

func TestValidateLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		wantErr string
	}{
		{name: "empty allowed (defaults)", level: ""},
		{name: "debug accepted", level: "debug"},
		{name: "info accepted", level: "info"},
		{name: "warn accepted", level: "warn"},
		{name: "error accepted", level: "error"},
		{name: "invalid rejected", level: "loud", wantErr: "unknown log level"},
		{name: "typo rejected", level: "verbose", wantErr: "unknown log level"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.Global.LogLevel = tt.level
			err := ValidateConfig(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateConcurrency(t *testing.T) {
	tests := []struct {
		name        string
		concurrency int
		wantErr     string
	}{
		{name: "zero rejected", concurrency: 0, wantErr: "concurrency must be >= 1"},
		{name: "negative rejected", concurrency: -1, wantErr: "concurrency must be >= 1"},
		{name: "negative-large rejected", concurrency: -100, wantErr: "concurrency must be >= 1"},
		{name: "above max rejected", concurrency: MaxConcurrency + 1, wantErr: fmt.Sprintf("concurrency must be <= %d", MaxConcurrency)},
		{name: "way above max rejected", concurrency: 9999, wantErr: fmt.Sprintf("concurrency must be <= %d", MaxConcurrency)},
		{name: "lower bound accepted", concurrency: 1},
		{name: "default accepted", concurrency: 10},
		{name: "upper bound accepted", concurrency: MaxConcurrency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.Options.Concurrency = tt.concurrency
			err := ValidateConfig(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateTmpDir(t *testing.T) {
	t.Run("empty allowed", func(t *testing.T) {
		cfg := validBaseConfig()
		cfg.Options.TmpDir = ""
		require.NoError(t, ValidateConfig(cfg))
	})

	t.Run("existing directory accepted", func(t *testing.T) {
		cfg := validBaseConfig()
		cfg.Options.TmpDir = t.TempDir()
		require.NoError(t, ValidateConfig(cfg))
	})

	t.Run("nonexistent path rejected", func(t *testing.T) {
		cfg := validBaseConfig()
		cfg.Options.TmpDir = "/nonexistent/path/tmp"
		err := ValidateConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tmp_dir")
		assert.Contains(t, err.Error(), "directory does not exist")
	})

	t.Run("file rejected", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "afile")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
		cfg := validBaseConfig()
		cfg.Options.TmpDir = f
		err := ValidateConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tmp_dir")
		assert.Contains(t, err.Error(), "path is not a directory")
	})
}

func TestValidateConfig_StopsOnFirstError(t *testing.T) {
	// Confirms ValidateConfig short-circuits: a nil structure should be reported
	// before field-level errors are evaluated.
	err := ValidateConfig(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
	assert.NotContains(t, err.Error(), "concurrency")
}

func TestValidatePackageNames(t *testing.T) {
	packages := []Package{
		{Name: "core"},
		{Name: "nginx"},
		{Name: "podinfo"},
	}

	tests := []struct {
		name            string
		names           []string
		wantErrContains []string
	}{
		{
			name:  "nil names passes",
			names: nil,
		},
		{
			name:  "empty names passes",
			names: []string{},
		},
		{
			name:  "valid single name",
			names: []string{"nginx"},
		},
		{
			name:  "valid multiple names",
			names: []string{"core", "podinfo"},
		},
		{
			name:            "unknown package name includes available packages",
			names:           []string{"bogus"},
			wantErrContains: []string{"unknown packages", "available packages"},
		},
		{
			name:            "mix of valid and invalid",
			names:           []string{"core", "invalid"},
			wantErrContains: []string{"unknown packages", "available packages"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageNames(tt.names, packages)
			if len(tt.wantErrContains) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, s := range tt.wantErrContains {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}
