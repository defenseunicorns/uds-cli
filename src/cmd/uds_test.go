// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/spf13/cobra"
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

func TestPublishForceUploadFlag(t *testing.T) {
	flag := publishCmd.Flags().Lookup("force-upload")
	require.NotNil(t, flag)
	require.Equal(t, "false", flag.DefValue)
}

func TestApplyPublishForceUploadEnv(t *testing.T) {
	originalPublishOpts := bundleCfg.PublishOpts
	t.Cleanup(func() {
		bundleCfg.PublishOpts = originalPublishOpts
	})

	tests := []struct {
		name          string
		envValue      string
		initialValue  bool
		flagValue     string
		expectedValue bool
		wantErr       bool
	}{
		{
			name:          "enables force upload from env",
			envValue:      "true",
			expectedValue: true,
		},
		{
			name:          "disables force upload from env",
			envValue:      "false",
			initialValue:  true,
			expectedValue: false,
		},
		{
			name:          "explicit flag overrides env",
			envValue:      "true",
			flagValue:     "false",
			expectedValue: false,
		},
		{
			name:          "explicit enabled flag overrides env",
			envValue:      "false",
			flagValue:     "true",
			expectedValue: true,
		},
		{
			name:     "invalid env value errors",
			envValue: "sometimes",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().BoolVar(&bundleCfg.PublishOpts.ForceUpload, "force-upload", false, "")
			bundleCfg.PublishOpts.ForceUpload = tt.initialValue
			t.Setenv(publishForceUploadEnvVar, tt.envValue)

			if tt.flagValue != "" {
				require.NoError(t, cmd.Flags().Set("force-upload", tt.flagValue))
			}

			err := applyPublishForceUploadEnv(cmd)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), publishForceUploadEnvVar)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedValue, bundleCfg.PublishOpts.ForceUpload)
		})
	}
}
