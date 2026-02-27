// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
)

func TestResolvePackageTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		pkg           types.Package
		expected      time.Duration
		expectedError string
	}{
		{
			name:     "empty timeout uses default",
			pkg:      types.Package{Name: "podinfo"},
			expected: config.HelmTimeout,
		},
		{
			name:     "whitespace timeout uses default",
			pkg:      types.Package{Name: "podinfo", Timeout: "   "},
			expected: config.HelmTimeout,
		},
		{
			name:     "valid timeout parses",
			pkg:      types.Package{Name: "podinfo", Timeout: "2m30s"},
			expected: 2*time.Minute + 30*time.Second,
		},
		{
			name:     "valid timeout trims whitespace",
			pkg:      types.Package{Name: "podinfo", Timeout: " 45s "},
			expected: 45 * time.Second,
		},
		{
			name:          "invalid timeout fails",
			pkg:           types.Package{Name: "podinfo", Timeout: "ten-minutes"},
			expectedError: "invalid timeout for package \"podinfo\": \"ten-minutes\"",
		},
		{
			name:          "zero timeout fails",
			pkg:           types.Package{Name: "podinfo", Timeout: "0s"},
			expectedError: "timeout must be greater than zero",
		},
		{
			name:          "negative timeout fails",
			pkg:           types.Package{Name: "podinfo", Timeout: "-1s"},
			expectedError: "timeout must be greater than zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			timeout, err := resolvePackageTimeout(tt.pkg)
			if tt.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, timeout)
		})
	}
}

func TestValidatePackageTimeouts(t *testing.T) {
	t.Parallel()

	err := validatePackageTimeouts([]types.Package{
		{Name: "nginx", Timeout: "5m"},
		{Name: "podinfo", Timeout: "not-a-duration"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid timeout for package \"podinfo\": \"not-a-duration\"")
}

func TestFormPkgMetaTimeout(t *testing.T) {
	t.Parallel()

	metaWithTimeout := formPkgMeta(types.Package{
		Name:       "podinfo",
		Ref:        "0.0.1",
		Repository: "ghcr.io/example/podinfo",
		Timeout:    "5m",
	})

	require.Equal(t, "5m", metaWithTimeout["timeout"])

	metaWithoutTimeout := formPkgMeta(types.Package{
		Name: "podinfo",
		Ref:  "0.0.1",
		Path: "../../packages/podinfo",
	})

	_, exists := metaWithoutTimeout["timeout"]
	require.False(t, exists)
}
