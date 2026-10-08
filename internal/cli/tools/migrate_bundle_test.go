// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package tools

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestMigrationSkillCommand(t *testing.T) {
	t.Parallel()

	streams, _, out, _ := iostreams.NewTestIOStreams()
	cmd := NewToolsCommand(streams)
	cmd.SetArgs([]string{"migrate-bundle", "skill"})

	require.NoError(t, cmd.Execute())
	require.Equal(t, migrationSkill, out.String())
}

func TestMigrationSkillCommandRejectsArguments(t *testing.T) {
	t.Parallel()

	streams, _, _, _ := iostreams.NewTestIOStreams()
	cmd := NewToolsCommand(streams)
	cmd.SetArgs([]string{"migrate-bundle", "skill", "unexpected"})

	require.EqualError(t, cmd.Execute(), "unknown command \"unexpected\" for \"tools migrate-bundle skill\"")
}
