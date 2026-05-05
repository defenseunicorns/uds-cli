// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		name    string
		names   []string
		wantErr string
	}{
		{
			name:    "nil names passes",
			names:   nil,
			wantErr: "",
		},
		{
			name:    "empty names passes",
			names:   []string{},
			wantErr: "",
		},
		{
			name:    "valid single name",
			names:   []string{"nginx"},
			wantErr: "",
		},
		{
			name:    "valid multiple names",
			names:   []string{"core", "podinfo"},
			wantErr: "",
		},
		{
			name:    "unknown package name",
			names:   []string{"bogus"},
			wantErr: "unknown packages",
		},
		{
			name:    "mix of valid and invalid",
			names:   []string{"core", "invalid"},
			wantErr: "unknown packages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageNames(tt.names, packages)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

// TestValidateRemovalSafety covers the public dependency-safety check the CLI
// invokes from RemoveOptions.Validate(). The fixture below mirrors a typical
// UDS Core layering:
//
//	core         (no deps)
//	nginx        depends on core
//	podinfo      depends on nginx, core
//	standalone   (no deps, unrelated)
func TestValidateRemovalSafety(t *testing.T) {
	b := bundleWith(
		pkg("core"),
		pkg("nginx", "core"),
		pkg("podinfo", "nginx", "core"),
		pkg("standalone"),
	)

	tests := []struct {
		name    string
		remove  []string
		wantErr string
		// wantContains lets us assert specific blocker phrasing without
		// pinning the entire (multiline) error string.
		wantContains []string
	}{
		{
			name:   "empty filter is safe (full bundle removal)",
			remove: nil,
		},
		{
			name:   "leaf package is safe",
			remove: []string{"podinfo"},
		},
		{
			name:   "isolated package is safe",
			remove: []string{"standalone"},
		},
		{
			name:   "removing root with dependents is blocked",
			remove: []string{"core"},
			wantContains: []string{
				"cannot remove package(s) with bundle dependents",
				`"core" is required by: nginx, podinfo`,
				"--force",
			},
		},
		{
			name:   "removing middle blocks via upper level",
			remove: []string{"nginx"},
			wantContains: []string{`"nginx" is required by: podinfo`},
		},
		{
			name:   "removing all dependents along with root is safe",
			remove: []string{"core", "nginx", "podinfo"},
		},
		{
			name:   "partial chain still has a blocker",
			remove: []string{"core", "nginx"},
			wantContains: []string{
				`"core" is required by: podinfo`,
				`"nginx" is required by: podinfo`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRemovalSafety(b, tt.remove)
			if len(tt.wantContains) == 0 && tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, s := range tt.wantContains {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}

// TestFormatBlockersError verifies sort stability and message shape. The
// formatter underpins ValidateRemovalSafety's user-facing output.
func TestFormatBlockersError(t *testing.T) {
	blockers := map[string][]string{
		"core":  {"nginx", "podinfo"},
		"nginx": {"podinfo"},
	}
	err := formatBlockersError(blockers)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "cannot remove package(s) with bundle dependents:")
	assert.Contains(t, msg, `"core" is required by: nginx, podinfo`)
	assert.Contains(t, msg, `"nginx" is required by: podinfo`)
	assert.Contains(t, msg, "re-run with --force to override")
	// "core" line should appear before "nginx" line (sorted).
	assert.Less(t, strings.Index(msg, `"core"`), strings.Index(msg, `"nginx"`))
}
