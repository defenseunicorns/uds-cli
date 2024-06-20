// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
)

func TestValidateDevDeployFlags(t *testing.T) {
	testCases := []struct {
		name          string
		localBundle   bool
		DevDeployOpts types.BundleDevDeployOptions
		expectError   bool
	}{
		{
			name:        "Local bundle with --ref flag",
			localBundle: true,
			DevDeployOpts: types.BundleDevDeployOptions{
				Ref: map[string]string{"some-key": "some-ref"},
			},
			expectError: true,
		},
		{
			name:        "Remote bundle with --ref flag",
			localBundle: false,
			DevDeployOpts: types.BundleDevDeployOptions{
				Ref: map[string]string{"some-key": "some-ref"},
			},
			expectError: false,
		},
		{
			name:        "Local bundle with --flavor flag",
			localBundle: true,
			DevDeployOpts: types.BundleDevDeployOptions{
				Flavor: map[string]string{"some-key": "some-flavor"},
			},
			expectError: false,
		},
		{
			name:        "Remote bundle with --flavor flag",
			localBundle: false,
			DevDeployOpts: types.BundleDevDeployOptions{
				Flavor: map[string]string{"some-key": "some-flavor"},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bundleCfg.DevDeployOpts = tc.DevDeployOpts

			err := validateDevDeployFlags(tc.localBundle)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIsLocalBundle(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "Test with directory",
			src:  "../cmd/",
			want: true,
		},
		{
			name: "Test with .tar.zst file",
			src:  "/path/to/file.tar.zst",
			want: true,
		},
		{
			name: "Test with other file",
			src:  "/path/to/file.txt",
			want: false,
		},
		{
			name: "Test with registry",
			src:  "ghcr.io/defenseunicorns/uds-cli/nginx",
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := isLocalBundle(tc.src)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestPopulateFlavorMap(t *testing.T) {
	testCases := []struct {
		name        string
		FlavorInput string
		expect      map[string]string
		expectError bool
	}{
		{
			name:        "Test with valid flavor input",
			FlavorInput: "key1=value1,key2=value2",
			expect:      map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name:        "Test with single value",
			FlavorInput: "value1",
			expect:      map[string]string{"": "value1"},
		},
		{
			name:        "Test with invalid flavor input",
			FlavorInput: "key1=value1,key2",
			expectError: true,
		},
		{
			name:        "Test with empty flavor input",
			FlavorInput: "",
			expect:      nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bundleCfg.DevDeployOpts.FlavorInput = tc.FlavorInput
			bundleCfg.DevDeployOpts.Flavor = nil
			err := populateFlavorMap()
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expect, bundleCfg.DevDeployOpts.Flavor)
			}
		})
	}
}
