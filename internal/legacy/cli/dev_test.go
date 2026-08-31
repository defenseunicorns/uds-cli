// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zarfconfig "github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

func TestNewDevCommandContainsDisassemble(t *testing.T) {
	cmd := newDevCommand()
	found, _, err := cmd.Find([]string{"disassemble"})
	require.NoError(t, err)
	assert.Equal(t, "disassemble", found.Name())
	assert.Equal(t, "disassemble <source> <output-dir>", found.Use)
}

func TestLegacyDisassembleOptionsHonorSignatureBypass(t *testing.T) {
	previous := config.CommonOptions.SkipSignatureValidation
	t.Cleanup(func() { config.CommonOptions.SkipSignatureValidation = previous })

	config.CommonOptions.SkipSignatureValidation = false
	assert.Equal(t, layout.VerifyIfPossible, legacyDisassembleOptions("source", "output").VerificationStrategy)
	config.CommonOptions.SkipSignatureValidation = true
	assert.Equal(t, layout.VerifyNever, legacyDisassembleOptions("source", "output").VerificationStrategy)
}

func TestConfigureZarfUsesLegacyTempDirectory(t *testing.T) {
	previousZarfOptions := zarfconfig.CommonOptions
	previousTmpDir := config.CommonOptions.TempDirectory
	t.Cleanup(func() {
		zarfconfig.CommonOptions = previousZarfOptions
		config.CommonOptions.TempDirectory = previousTmpDir
	})

	config.CommonOptions.TempDirectory = "/tmp/legacy-disassemble"
	configureZarf()
	assert.Equal(t, config.CommonOptions.TempDirectory, zarfconfig.CommonOptions.TempDirectory)
}

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
			expectError: false,
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
			src:  ".",
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
