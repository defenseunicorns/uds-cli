// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
)

func TestValidatePackageNamespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		pkg           types.Package
		expectedError string
	}{
		{
			name: "empty namespace allowed",
			pkg:  types.Package{Name: "nginx"},
		},
		{
			name: "valid namespace",
			pkg:  types.Package{Name: "nginx", Namespace: "package-override-ns"},
		},
		{
			name:          "uppercase invalid",
			pkg:           types.Package{Name: "nginx", Namespace: "MyNamespace"},
			expectedError: "invalid namespace for package \"nginx\": \"MyNamespace\"",
		},
		{
			name:          "spaces invalid",
			pkg:           types.Package{Name: "nginx", Namespace: "my namespace"},
			expectedError: "invalid namespace for package \"nginx\": \"my namespace\"",
		},
		{
			name:          "leading hyphen invalid",
			pkg:           types.Package{Name: "nginx", Namespace: "-bad-namespace"},
			expectedError: "invalid namespace for package \"nginx\": \"-bad-namespace\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validatePackageNamespace(tt.pkg)
			if tt.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestValidatePackageNamespaces(t *testing.T) {
	t.Parallel()

	err := validatePackageNamespaces([]types.Package{
		{Name: "nginx", Namespace: "valid-namespace"},
		{Name: "podinfo", Namespace: "bad namespace"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid namespace for package \"podinfo\": \"bad namespace\"")
}
