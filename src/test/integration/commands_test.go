// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package integration

import (
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestVersion(t *testing.T) {
	t.Parallel()

	result := testutil.RunCLI(t, isolatedOptions(t, nil), "version")
	require.NoError(t, result.Err, result.Stderr)
	require.NotEmpty(t, result.Stdout)
}

func TestRootCommand(t *testing.T) {
	t.Parallel()

	result := testutil.RunCLI(t, isolatedOptions(t, nil))
	require.NoError(t, result.Err, result.Stderr)
	require.Contains(t, result.Stdout+result.Stderr, "UDS CLI")
}

func isolatedOptions(t *testing.T, overrides map[string]string) testutil.CommandOptions {
	t.Helper()

	env := map[string]string{
		"UDS_UDS_CACHE": filepath.Join(t.TempDir(), "cache"),
	}
	for key, value := range overrides {
		env[key] = value
	}
	return testutil.CommandOptions{Env: env}
}
