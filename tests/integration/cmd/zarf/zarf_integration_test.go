// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package zarf_test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

func TestZarfToolsIgnoreCLIFeatures_NextMode(t *testing.T) {
	uds := testutil.UDSCLIPath(t, "run via 'maru run test:next-integration'")
	t.Setenv("CLI_FEATURES", "NextMode=true")

	output, err := exec.Command(uds, "zarf", "tools", "kubectl", "version", "--client").CombinedOutput()
	require.NoError(t, err, string(output))
	require.Contains(t, string(output), "Client Version")
}

func TestZarfAliases_NextMode(t *testing.T) {
	uds := testutil.UDSCLIPath(t, "run via 'maru run test:next-integration'")
	t.Setenv("CLI_FEATURES", "NextMode=true")

	for _, args := range [][]string{
		{"z", "tools", "kubectl", "version", "--client"},
		{"tools", "z", "tools", "kubectl", "version", "--client"},
	} {
		output, err := exec.Command(uds, args...).CombinedOutput()
		require.NoError(t, err, string(output))
		require.Contains(t, string(output), "Client Version")
	}
}

func TestZarfToolsPreserveZarfFeatures_NextMode(t *testing.T) {
	uds := testutil.UDSCLIPath(t, "run via 'maru run test:next-integration'")

	output, err := exec.Command(uds, "--features=NextMode", "zarf", "--features=values=true", "tools", "kubectl", "version", "--client").CombinedOutput()
	require.NoError(t, err, string(output))
	require.Contains(t, string(output), "Client Version")
}
