// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package spec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestUDSBundleValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		bundle  UDSBundle
		wantErr string
	}{
		{
			name: "valid bundle",
			bundle: UDSBundle{
				UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
				Metadata: Metadata{Name: "example"},
				Packages: []Package{{Name: "app", Source: "oci://example.com/app:v1"}},
			},
		},
		{name: "missing required fields", wantErr: "uds.bundle_api_version is required"},
		{
			name: "unknown dependency",
			bundle: UDSBundle{
				UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
				Metadata: Metadata{Name: "example"},
				Packages: []Package{{Name: "app", Source: "oci://example.com/app:v1", DependsOn: []PackageRef{{Name: "missing"}}}},
			},
			wantErr: `package "app": depends_on references unknown package "missing"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.bundle.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestPackageSourceRangeOmittedFromSerialization(t *testing.T) {
	t.Parallel()

	pkg := Package{
		Name:   "app",
		Source: "oci://example.com/app:v1",
		SourceRange: SourceRange{
			Filename: "bundle.uds.hcl",
			Start:    SourcePosition{Line: 7, Column: 1, Byte: 42},
			End:      SourcePosition{Line: 7, Column: 17, Byte: 58},
		},
	}

	jsonData, err := json.Marshal(pkg)
	require.NoError(t, err)
	assert.NotContains(t, string(jsonData), "SourceRange")
	assert.NotContains(t, string(jsonData), "sourceRange")
	assert.NotContains(t, string(jsonData), "bundle.uds.hcl")

	yamlData, err := yaml.Marshal(pkg)
	require.NoError(t, err)
	assert.NotContains(t, string(yamlData), "SourceRange")
	assert.NotContains(t, string(yamlData), "sourceRange")
	assert.NotContains(t, string(yamlData), "bundle.uds.hcl")
}
