// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/stretchr/testify/require"
	zarfapi "github.com/zarf-dev/zarf/src/api"
)

// validValidationConfig returns baseline valid Zarf configuration for tests.
func validValidationConfig() *UDSBundleConfig {
	return &UDSBundleConfig{
		Options: &bundleinternal.ConfigOptions{Concurrency: 10},
	}
}

func TestDeployPackageOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    DeployPackageOptions
		wantErr string
	}{
		{name: "nil config", wantErr: "config is required"},
		{name: "empty bundle directory", opts: DeployPackageOptions{Config: validValidationConfig()}, wantErr: "bundle directory is required"},
		{name: "valid", opts: DeployPackageOptions{Config: validValidationConfig(), BundleDir: "/bundle"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidatePackageFileChecksums(t *testing.T) {
	const sha256Digest = "0000000000000000000000000000000000000000000000000000000000000000"
	tests := []struct {
		name     string
		checksum string
		wantErr  bool
	}{
		{name: "unset"},
		{name: "unprefixed SHA-256", checksum: sha256Digest},
		{name: "prefixed SHA-256", checksum: "sha256:" + sha256Digest},
		{name: "empty digest", checksum: "sha256:", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := zarfapi.Package{Components: []zarfapi.Component{{
				Name:  "main",
				Files: []zarfapi.File{{Source: "file.txt", Checksum: zarfapi.ParseFileChecksum(tt.checksum)}},
			}}}

			err := validatePackageFileChecksums(pkg)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrInvalidPackageFileChecksum)
			require.ErrorContains(t, err, `component "main" file "file.txt" has an empty digest`)
		})
	}
}

func TestRemovePackageOptionsValidate(t *testing.T) {
	require.ErrorContains(t, (RemovePackageOptions{}).Validate(), "config is required")
	require.NoError(t, (RemovePackageOptions{Config: validValidationConfig()}).Validate())
}
