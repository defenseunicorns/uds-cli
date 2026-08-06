// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewCommand(t *testing.T) {
	t.Setenv("UDS_CONFIG", "")
	t.Chdir(t.TempDir())
	root, err := NewCommand()
	require.NoError(t, err)

	require.NotNil(t, root.PersistentFlags().Lookup("cli-mode"))
	for _, name := range []string{
		"completion", "create", "deploy", "dev", "inspect", "internal", "list",
		"logs", "monitor", "publish", "pull", "remove", "run", "version", "zarf",
	} {
		cmd, _, err := root.Find([]string{name})
		require.NoError(t, err, name)
		require.Equal(t, name, cmd.Name())
	}
}

func TestNewCommandReturnsConfigError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "uds-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("options:\n  invalid: true\n"), 0o600))
	t.Setenv("UDS_CONFIG", configPath)

	root, err := NewCommand()
	require.Nil(t, root)
	require.ErrorContains(t, err, "failed to load uds-config")
	require.ErrorContains(t, err, "invalid config option: invalid")
}

func TestNewCommandDoesNotReuseViper(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "uds-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("options:\n  log_level: debug\n"), 0o600))
	t.Setenv("UDS_CONFIG", configPath)

	first, err := NewCommand()
	require.NoError(t, err)
	require.Equal(t, configPath, v.ConfigFileUsed())

	t.Setenv("UDS_CONFIG", "")
	t.Chdir(t.TempDir())
	second, err := NewCommand()
	require.NoError(t, err)
	require.Empty(t, v.ConfigFileUsed())
	require.NotSame(t, first, second)
}
