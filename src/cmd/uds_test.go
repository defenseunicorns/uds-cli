// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalAndValidateConfig(t *testing.T) {
	tests := []struct {
		name        string
		configFile  []byte
		bundleCfg   *types.BundleConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "Invalid option key",
			configFile: []byte(`
options:
  log_levelx: debug
`),
			bundleCfg: &types.BundleConfig{},

			wantErr:     true,
			errContains: "invalid config option: log_levelx",
		},
		{
			name: "Option typo",
			configFile: []byte(`
optionx:
  log_level: debug
`),
			bundleCfg: &types.BundleConfig{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := unmarshalAndValidateConfig(tt.configFile, tt.bundleCfg)
			if tt.wantErr {
				require.NotNil(t, err, "Expected error")
				require.Contains(t, err.Error(), tt.errContains, "Error message should contain the expected string")
			} else {
				require.Nil(t, err, "Expected no error")
			}
		})
	}
}
