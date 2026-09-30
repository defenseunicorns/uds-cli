package main

import (
	"os"
	"path/filepath"
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
	require.Equal(t, "Instructions.\n", string(first))
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
