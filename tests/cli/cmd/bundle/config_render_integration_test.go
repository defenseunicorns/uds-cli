// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package bundle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/internal/cli"
	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestConfigRenderCLI(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "config.uds.hcl")
	original := []byte("variables = { secret = \"value\" }\n")
	require.NoError(t, os.WriteFile(input, original, 0o600))
	run := func(args ...string) (string, error) {
		streams, _, out, _ := iostreams.NewTestIOStreams()
		root := cli.NewRootCommand(streams, mode.FeatureSet{})
		root.SetArgs(append([]string{"bundle", "config", "render"}, args...))
		err := root.Execute()
		return out.String(), err
	}
	stdout, err := run(input)
	require.NoError(t, err)
	require.Contains(t, stdout, "secret")
	require.Contains(t, stdout, "value")

	output := filepath.Join(dir, "rendered.hcl")
	got, err := run(input, "--output-file", output)
	require.NoError(t, err)
	require.Empty(t, got)
	contents, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, stdout, string(contents))
	secondOutput := filepath.Join(dir, "second.hcl")
	got, err = run("--output-file", secondOutput, input)
	require.NoError(t, err)
	require.Empty(t, got)
	secondContents, err := os.ReadFile(secondOutput)
	require.NoError(t, err)
	require.Equal(t, stdout, string(secondContents))
	info, err := os.Stat(output)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	require.NoError(t, os.WriteFile(output, []byte("preserve"), 0o644))
	_, err = run(input, "--output-file", output)
	require.Error(t, err)
	contents, err = os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "preserve", string(contents))
	_, err = run(input, "--output-file", output, "--force")
	require.NoError(t, err)
	info, err = os.Stat(output)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	hardlink := filepath.Join(dir, "hardlink")
	symlink := filepath.Join(dir, "symlink")
	require.NoError(t, os.Link(input, hardlink))
	require.NoError(t, os.Symlink(input, symlink))
	for _, path := range []string{input, filepath.Join(dir, ".", "config.uds.hcl"), hardlink, symlink} {
		_, err = run(input, "--output-file", path, "--force")
		require.Error(t, err)
		contents, err = os.ReadFile(input)
		require.NoError(t, err)
		require.Equal(t, original, contents)
	}

	bad := filepath.Join(dir, "bad.hcl")
	require.NoError(t, os.WriteFile(bad, []byte("variables = {"), 0o600))
	before, err := os.ReadFile(output)
	require.NoError(t, err)
	got, err = run(bad, "--output-file", output, "--force")
	require.Error(t, err)
	require.Empty(t, got)
	contents, err = os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, before, contents)
	got, err = run(bad)
	require.Error(t, err)
	require.Empty(t, got)
	_, err = run()
	require.Error(t, err)
	_, err = run(input, "--output-file=")
	require.Error(t, err)
}

func TestConfigRenderCLIRelativeFileSnapshot(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "config.uds.hcl")
	source := filepath.Join(dir, "secret.txt")
	output := filepath.Join(dir, "rendered.uds.hcl")
	value := "literal ${not_a_variable} and %{ not_a_directive }\n"
	require.NoError(t, os.WriteFile(source, []byte(value), 0o600))
	require.NoError(t, os.WriteFile(input, []byte("variables = { secret = file(\"secret.txt\") }\n"), 0o600))

	run := func(args ...string) (string, error) {
		t.Helper()
		streams, _, out, _ := iostreams.NewTestIOStreams()
		root := cli.NewRootCommand(streams, mode.FeatureSet{})
		root.SetArgs(append([]string{"bundle", "config", "render"}, args...))
		err := root.ExecuteContext(t.Context())
		return out.String(), err
	}
	assertSnapshot := func(snapshot string) {
		t.Helper()
		file, diags := hclsyntax.ParseConfig([]byte(snapshot), "snapshot.hcl", hcl.Pos{Line: 1, Column: 1})
		require.False(t, diags.HasErrors(), "%s", diags.Error())
		body, ok := file.Body.(*hclsyntax.Body)
		require.True(t, ok)
		variables, ok := body.Attributes["variables"]
		require.True(t, ok)
		result, diags := variables.Expr.Value(nil)
		require.False(t, diags.HasErrors(), "%s", diags.Error())
		require.Equal(t, value, result.GetAttr("secret").AsString())
	}

	stdout, err := run(input)
	require.NoError(t, err)
	assertSnapshot(stdout)
	stdout, err = run(input, "--output-file", output)
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.NoError(t, os.Remove(source))
	snapshot, err := os.ReadFile(output)
	require.NoError(t, err)
	assertSnapshot(string(snapshot))
	stdout, err = run(output)
	require.NoError(t, err)
	assertSnapshot(stdout)
}

func TestConfigRenderRejectsInheritedOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "config.uds.hcl")
	require.NoError(t, os.WriteFile(input, []byte("variables = { secret = \"value\" }\n"), 0o600))
	output := filepath.Join(dir, "rendered.hcl")
	for _, args := range [][]string{
		{"bundle", "--output", output, "config", "render", input},
		{"bundle", "config", "--output", output, "render", input},
		{"bundle", "-o", output, "config", "render", input, "--output-file", filepath.Join(dir, "other.hcl")},
		{"bundle", "config", "render", input, "--output", output},
	} {
		streams, _, out, _ := iostreams.NewTestIOStreams()
		root := cli.NewRootCommand(streams, mode.FeatureSet{})
		root.SetArgs(args)
		err := root.Execute()
		require.ErrorContains(t, err, "--output is not supported by config render; use --output-file")
		require.Empty(t, out.String())
		_, err = os.Stat(output)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
}

func TestConfigRenderHelp(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()
	root := cli.NewRootCommand(streams, mode.FeatureSet{})
	root.SetArgs([]string{"bundle", "config", "render", "--help"})
	require.NoError(t, root.Execute())
	for _, text := range []string{"static HCL", "without merging defaults", "file()", "plaintext secrets", "--output-file"} {
		require.Contains(t, out.String(), text)
	}
}
