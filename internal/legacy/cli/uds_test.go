// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
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
				require.Error(t, err, "Expected error")
				require.Contains(t, err.Error(), tt.errContains, "Error message should contain the expected string")
			} else {
				require.NoError(t, err, "Expected no error")
			}
		})
	}
}

func TestPublishForceUploadFlag(t *testing.T) {
	root := NewRootCommand()
	publishCmd, _, err := root.Find([]string{"publish"})
	require.NoError(t, err)
	flag := publishCmd.Flags().Lookup("force-upload")
	require.NotNil(t, flag)
	require.Equal(t, "false", flag.DefValue)
}

func TestDeployArchitectureOverrideRequiresCLIOptIn(t *testing.T) {
	t.Setenv("UDS_SKIP_ARCHITECTURE_CHECK", "true")
	t.Setenv("UDS_DEPLOY_SKIP_ARCHITECTURE_CHECK", "true")
	for _, path := range [][]string{{"deploy"}, {"dev", "deploy"}} {
		t.Run(strings.Join(path, "/"), func(t *testing.T) {
			root := NewRootCommand()
			v.Set("deploy.skip-architecture-check", true)
			cmd, _, err := root.Find(path)
			require.NoError(t, err)
			require.NoError(t, applyViperFlags(cmd))
			require.False(t, bundleCfg.DeployOpts.SkipArchitectureCheck)
			require.Equal(t, "false", cmd.Flags().Lookup("skip-architecture-check").DefValue)
			require.NoError(t, cmd.ParseFlags([]string{"--skip-architecture-check"}))
			require.True(t, bundleCfg.DeployOpts.SkipArchitectureCheck)
			require.NoError(t, unmarshalAndValidateConfig([]byte("retries: 2\n"), &bundleCfg))
			require.True(t, bundleCfg.DeployOpts.SkipArchitectureCheck)
			require.NoError(t, cmd.ParseFlags([]string{"--skip-architecture-check=false"}))
			require.False(t, bundleCfg.DeployOpts.SkipArchitectureCheck)
			require.Error(t, unmarshalAndValidateConfig([]byte("skiparchitecturecheck: true\n"), &bundleCfg))
			require.False(t, bundleCfg.DeployOpts.SkipArchitectureCheck)
		})
	}
}
