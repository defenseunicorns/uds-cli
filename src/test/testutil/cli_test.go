// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCLIPathRequiresConfiguredBinary(t *testing.T) {
	t.Setenv("UDS_CLI_PATH", "")
	require.Panics(t, func() { CLIPath(t) })
}

func TestRunCLIUsesConfiguredBinaryAndChildEnvironment(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "uds")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s' \"$CHILD_ONLY\"\nprintf '%s' \"$1\" >&2\n"), 0o700))
	t.Setenv("UDS_CLI_PATH", binary)
	t.Setenv("CHILD_ONLY", "")

	result := RunCLI(t, CommandOptions{Env: map[string]string{"CHILD_ONLY": "set-for-child"}}, "argument")

	require.NoError(t, result.Err)
	require.Equal(t, "set-for-child", result.Stdout)
	require.Equal(t, "argument", result.Stderr)
	require.Empty(t, os.Getenv("CHILD_ONLY"))
}

func TestRunCLIUsesExplicitWorkingDirectory(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "uds")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\npwd\n"), 0o700))
	t.Setenv("UDS_CLI_PATH", binary)
	dir := t.TempDir()
	parentDir, err := os.Getwd()
	require.NoError(t, err)

	result := RunCLI(t, CommandOptions{Dir: dir})

	require.NoError(t, result.Err)
	resolvedDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	require.Equal(t, resolvedDir+"\n", result.Stdout)
	afterRunDir, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, parentDir, afterRunDir)
}
