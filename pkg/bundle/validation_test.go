// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateConfig_Nil(t *testing.T) {
	err := ValidateConfig(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
}

func TestValidateConfig_NilGlobal(t *testing.T) {
	err := ValidateConfig(&UDSBundleConfig{
		Options: &ConfigOptions{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config.Global is required")
}

func TestValidateConfig_NilOptions(t *testing.T) {
	err := ValidateConfig(&UDSBundleConfig{
		Global: &GlobalOptions{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config.Options is required")
}

func TestValidateConfig_Valid(t *testing.T) {
	err := ValidateConfig(&UDSBundleConfig{
		Global:  &GlobalOptions{},
		Options: &ConfigOptions{},
	})
	require.NoError(t, err)
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
