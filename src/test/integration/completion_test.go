// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package integration

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestCompletion(t *testing.T) {
	t.Parallel()

	result := testutil.RunCLI(t, isolatedOptions(t, nil), "completion", "bash")
	require.NoError(t, result.Err, result.Stderr)
	require.Contains(t, result.Stdout, "bash completion V2 for uds")
}

func TestCompletionHelp(t *testing.T) {
	t.Parallel()

	result := testutil.RunCLI(t, isolatedOptions(t, nil), "completion")
	require.NoError(t, result.Err, result.Stderr)
	require.Contains(t, result.Stdout, "Generate the autocompletion script for uds for the specified shell.")
}

func TestShellCompletion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		shell string
		want  string
	}{
		{name: "bash", shell: "bash", want: "bash completion V2 for uds"},
		{name: "zsh", shell: "zsh", want: "zsh completion for uds"},
		{name: "fish", shell: "fish", want: "fish completion for uds"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := testutil.RunCLI(t, isolatedOptions(t, nil), "completion", tt.shell)
			require.NoError(t, result.Err, result.Stderr)
			require.Contains(t, result.Stdout, tt.want)
		})
	}
}
