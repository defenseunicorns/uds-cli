// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestMigrationPromptMatchesSkill(t *testing.T) {
	t.Parallel()

	skill, err := os.ReadFile("../../../.agents/skills/migrate-legacy-bundle-to-next/SKILL.md")
	require.NoError(t, err)
	frontmatter, ok := strings.CutPrefix(string(skill), "---\n")
	require.True(t, ok, "skill must begin with YAML frontmatter")
	_, body, ok := strings.Cut(frontmatter, "\n---\n")
	require.True(t, ok, "skill must have a closing frontmatter delimiter")
	require.Equal(t, strings.TrimSpace(body)+"\n", migrationPrompt,
		"run uds run generate:migration-prompt after updating the skill")
}

func TestMigrationPromptCommand(t *testing.T) {
	t.Parallel()

	streams, _, out, _ := iostreams.NewTestIOStreams()
	cmd := NewBundleCommand(streams)
	cmd.SetArgs([]string{"migrate", "prompt"})

	require.NoError(t, cmd.Execute())
	require.Equal(t, migrationPrompt, out.String())
}

func TestMigrationPromptCommandRejectsArguments(t *testing.T) {
	t.Parallel()

	streams, _, _, _ := iostreams.NewTestIOStreams()
	cmd := NewBundleCommand(streams)
	cmd.SetArgs([]string{"migrate", "prompt", "unexpected"})

	require.EqualError(t, cmd.Execute(), "unknown command \"unexpected\" for \"bundle migrate prompt\"")
}
