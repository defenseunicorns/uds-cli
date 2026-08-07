// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package integration

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestZarfLint(t *testing.T) {
	t.Parallel()

	packageDir := testutil.CopyFixture(t, "packages/podinfo")
	result := testutil.RunCLI(t, isolatedOptions(t, nil), "zarf", "dev", "lint", packageDir)
	require.NoError(t, result.Err, result.Stderr)
	require.Contains(t, result.Stdout+result.Stderr, "Image not pinned with digest - ghcr.io/stefanprodan/podinfo:6.4.0")
}

func TestZarfToolsVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tool string
		want string
	}{
		{name: "helm", tool: "helm", want: "version.BuildInfo"},
		// Crane is deprecated and its version is not embedded in local development builds.
		{name: "crane", tool: "crane"},
		{name: "syft", tool: "syft", want: "Application:"},
		{name: "archiver", tool: "archiver", want: "mholt/archives"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := testutil.RunCLI(t, isolatedOptions(t, nil), "zarf", "tools", tt.tool, "version")
			require.NoError(t, result.Err, result.Stderr)
			output := result.Stdout + result.Stderr
			require.NotEmpty(t, output)
			if tt.want != "" {
				require.Contains(t, output, tt.want)
			}
		})
	}
}
