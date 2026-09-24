// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestMigrationPromptCommand(t *testing.T) {
	t.Parallel()

	streams, _, out, _ := iostreams.NewTestIOStreams()
	cmd := NewBundleCommand(streams)
	cmd.SetArgs([]string{"migrate", "prompt"})

	require.NoError(t, cmd.Execute())
	require.Equal(t, migrationPrompt+"\n", out.String())
}

func TestMigrationPromptCommandRejectsArguments(t *testing.T) {
	t.Parallel()

	streams, _, _, _ := iostreams.NewTestIOStreams()
	cmd := NewBundleCommand(streams)
	cmd.SetArgs([]string{"migrate", "prompt", "unexpected"})

	require.EqualError(t, cmd.Execute(), "unknown command \"unexpected\" for \"bundle migrate prompt\"")
}
