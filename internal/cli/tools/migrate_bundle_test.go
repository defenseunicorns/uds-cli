// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package tools

import (
	"os"
	"path/filepath"
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

func TestMigrationSkillCommandWritesFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "migration-skill.md")
	streams, _, out, _ := iostreams.NewTestIOStreams()
	cmd := NewToolsCommand(streams)
	cmd.SetArgs([]string{"migrate-bundle", "skill", "-o", path})

	require.NoError(t, cmd.Execute())
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, migrationSkill, string(content))
	require.Empty(t, out.String())
}

func TestMigrationSkillCommandPreservesExistingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "migration-skill.md")
	require.NoError(t, os.WriteFile(path, []byte("existing content"), 0o600))
	streams, _, _, _ := iostreams.NewTestIOStreams()
	cmd := NewToolsCommand(streams)
	cmd.SetArgs([]string{"migrate-bundle", "skill", "--output", path})

	require.ErrorContains(t, cmd.Execute(), "create migration skill file")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "existing content", string(content))
}

func TestMigrationSkillCommandRejectsEmptyOutputPath(t *testing.T) {
	t.Parallel()

	streams, _, _, _ := iostreams.NewTestIOStreams()
	cmd := NewToolsCommand(streams)
	cmd.SetArgs([]string{"migrate-bundle", "skill", "-o", ""})

	require.EqualError(t, cmd.Execute(), "output path must not be empty")
}
