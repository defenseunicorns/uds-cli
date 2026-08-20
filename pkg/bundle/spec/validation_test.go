// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUDSBundleValidate_ErrorContracts(t *testing.T) {
	t.Parallel()

	err := (&UDSBundle{}).Validate()
	require.ErrorIs(t, err, ErrBundleAPIVersionRequired)
	require.ErrorIs(t, err, ErrMetadataNameRequired)
	require.ErrorIs(t, err, ErrPackagesRequired)

	bundle := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "example"},
		Packages: []Package{{Name: "app", Source: "oci://example.com/app:v1", DependsOn: []PackageRef{{Name: "missing"}}}},
	}
	err = bundle.Validate()
	var unknownDependency *UnknownDependencyError
	require.ErrorAs(t, err, &unknownDependency)
	assert.Equal(t, "app", unknownDependency.Package)
	assert.Equal(t, "missing", unknownDependency.Dependency)
}

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
