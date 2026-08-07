// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// validValidationConfig returns baseline valid Zarf configuration for tests.
func validValidationConfig() *UDSBundleConfig {
	return &UDSBundleConfig{
		Global:  &GlobalOptions{},
		Options: &ConfigOptions{Concurrency: 10},
	}
}

func TestDeployPackageOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    DeployPackageOptions
		wantErr string
	}{
		{name: "nil config", wantErr: "config is required"},
		{name: "empty bundle directory", opts: DeployPackageOptions{Config: validValidationConfig()}, wantErr: "BundleDir is required"},
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

func TestRemovePackageOptionsValidate(t *testing.T) {
	require.ErrorContains(t, (RemovePackageOptions{}).Validate(), "config is required")
	require.NoError(t, (RemovePackageOptions{Config: validValidationConfig()}).Validate())
}
