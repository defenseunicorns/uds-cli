// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSkillBody(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		skill string
		want  string
	}{
		{name: "body only", skill: "---\nname: migration\n---\n\nInstructions.\n", want: "Instructions.\n"},
		{name: "windows newlines", skill: "---\r\nname: migration\r\n---\r\nInstructions.\r\n", want: "Instructions.\n"},
		{name: "missing frontmatter", skill: "Instructions."},
		{name: "unclosed frontmatter", skill: "---\nname: migration\n"},
		{name: "empty body", skill: "---\nname: migration\n---\n\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := skillBody(tc.skill)
			if tc.want == "" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestGenerateAndCheck(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	skill := filepath.Join(root, skillPath)
	asset := filepath.Join(root, assetPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(skill), 0o755))
	require.NoError(t, os.WriteFile(skill, []byte("---\nname: migration\n---\nInstructions.\n"), 0o600))

	// A missing or stale asset must fail checks without being created or changed.
	require.Error(t, generate(root, true))
	_, err := os.Stat(asset)
	require.True(t, os.IsNotExist(err))
	require.NoError(t, generate(root, false))
	require.NoError(t, generate(root, true))
	first, err := os.ReadFile(asset)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(string(first), "Instructions.\n"))
	require.NoError(t, generate(root, false))
	second, err := os.ReadFile(asset)
	require.NoError(t, err)
	require.Equal(t, first, second)

	require.NoError(t, os.WriteFile(skill, []byte("---\nname: migration\n---\nUpdated instructions.\n"), 0o600))
	require.ErrorContains(t, generate(root, true), "migration prompt is stale")
	unchanged, err := os.ReadFile(asset)
	require.NoError(t, err)
	require.Equal(t, first, unchanged)
	require.NoError(t, generate(root, false))
	require.NoError(t, generate(root, true))
}

func TestDocumentationDependencies(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	skill := "---\nname: migration\n---\nRead [schema](docs/reference/schema.mdx), [schema again](docs/reference/schema.mdx), and [workflow](docs/how-to-guides/workflow.mdx).\n"
	writeSource := func(path, content string) {
		t.Helper()
		path = filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	writeSource(skillPath, skill)
	schema := "---\ntitle: Schema\n---\nSchema rules. See [workflow](/cli/how-to-guides/workflow/).\n"
	writeSource("docs/reference/schema.mdx", schema)
	writeSource("docs/how-to-guides/workflow.mdx", "---\ntitle: Workflow\n---\nWorkflow rules. See [schema](docs/reference/schema.mdx).\n")
	require.NoError(t, generate(root, false))
	asset := filepath.Join(root, assetPath)
	first, err := os.ReadFile(asset)
	require.NoError(t, err)
	prompt := string(first)
	require.Contains(t, prompt, "[schema](#migration-doc-docs-reference-schema-mdx)")
	require.Contains(t, prompt, "[workflow](#migration-doc-docs-how-to-guides-workflow-mdx)")
	require.Contains(t, prompt, "Schema rules.")
	require.Contains(t, prompt, "Workflow rules.")
	require.Equal(t, 1, strings.Count(prompt, "## Included documentation: docs/reference/schema.mdx"))
	require.Less(t, strings.Index(prompt, "Schema rules."), strings.Index(prompt, "Workflow rules."))
	require.NotContains(t, prompt, "title: Schema")
	require.NoError(t, generate(root, true))

	// Both body and metadata changes in an included doc must invalidate the asset.
	for _, update := range []string{
		strings.Replace(schema, "Schema rules.", "Updated schema rules.", 1),
		strings.Replace(schema, "title: Schema", "title: Renamed schema", 1),
	} {
		writeSource("docs/reference/schema.mdx", update)
		require.ErrorContains(t, generate(root, true), "migration prompt is stale")
		unchanged, err := os.ReadFile(asset)
		require.NoError(t, err)
		require.Equal(t, first, unchanged)
	}
	writeSource("docs/reference/schema.mdx", schema)
	require.NoError(t, generate(root, true))
	writeSource(skillPath, strings.Replace(skill, "name: migration", "name: renamed", 1))
	require.ErrorContains(t, generate(root, true), "migration prompt is stale")
}

func TestUnavailableDocumentation(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"docs/missing.mdx", "docs/../../outside.mdx", "docs/schema.txt"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			_, err := assemble(t.TempDir(), "---\nname: migration\n---\nRead [reference]("+path+").\n")
			require.Error(t, err)
		})
	}
}

func TestCommittedPromptIsCurrent(t *testing.T) {
	t.Parallel()
	require.NoError(t, generate("../..", true), "run uds run generate:migration-prompt")
}
