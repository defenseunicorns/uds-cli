// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package bundle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/cli"
	bundlecmd "github.com/defenseunicorns/uds-cli/internal/cli/bundle"
	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lintSource(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "bundle.uds.hcl"), []byte(`uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "lint-test" }
package "app" {
  source = "oci://invalid.example.test/app:1"
  values_files = ["${sys.arch}.yaml"]
}
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "defaults.uds.hcl"), []byte("variables = {}\n"), 0o600))
	return root
}

func lintCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	streams, _, out, errOut := iostreams.NewTestIOStreams()
	root := cli.NewRootCommand(streams, mode.FeatureSet{})
	root.SetArgs(args)
	err := root.ExecuteContext(t.Context())
	assert.Empty(t, errOut.String(), "only the executable should report final errors")
	return out.String(), err
}

func TestLintCommandEvaluationAndOptOut(t *testing.T) {
	root := lintSource(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "amd64.yaml"), []byte("host: {{ .vars.domain }}\n"), 0o600))
	output, err := lintCLI(t, "bundle", "lint", root, "--architecture", "amd64")
	require.ErrorIs(t, err, bundlecmd.ErrLintFailed)
	assert.Contains(t, output, "amd64.yaml:1:")
	assert.Contains(t, output, "--skip-template-evaluation")

	output, err = lintCLI(t, "bundle", "lint", root, "-a", "amd64", "--skip-template-evaluation")
	require.NoError(t, err)
	assert.Equal(t, "No issues found.\n", output)

	output, err = lintCLI(t, "bundle", "lint", filepath.Join(root, "bundle.uds.hcl"), "-a", "amd64", "-s", "domain=ci.example.test")
	require.NoError(t, err)
	assert.Equal(t, "No issues found.\n", output)
}

func TestLintCommandExplicitConfigAndWorkingDirectory(t *testing.T) {
	root := lintSource(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "amd64.yaml"), []byte("host: {{ .vars.domain }}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "arm64.yaml"), []byte("{{"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.uds.hcl"), []byte("not valid HCL"), 0o600))
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "dev.hcl"), []byte("options { architecture = \"amd64\" }\nvariables = { domain = file(\"domain.txt\") }\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "domain.txt"), []byte("dev.example.test"), 0o600))
	t.Chdir(configDir)

	output, err := lintCLI(t, "bundle", "--config", "dev.hcl", "lint", root)
	require.NoError(t, err)
	assert.Equal(t, "No issues found.\n", output)

	_, err = lintCLI(t, "bundle", "lint", root, "--config", "dev.hcl", "-a", "arm64")
	require.ErrorIs(t, err, bundlecmd.ErrLintFailed, "explicit architecture overrides config")

	_, err = lintCLI(t, "bundle", "lint", root, "-a", "amd64")
	require.ErrorIs(t, err, bundlecmd.ErrLintFailed, "adjacent config is ignored, so domain is missing")
}

func TestLintCommandDefaultsToCurrentDirectoryAndDoesNotRecurse(t *testing.T) {
	root := lintSource(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "amd64.yaml"), []byte("ok: true"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(root, "nested"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", "bundle.uds.hcl"), []byte("invalid"), 0o600))
	t.Chdir(root)
	output, err := lintCLI(t, "bundle", "lint", "-a", "amd64")
	require.NoError(t, err)
	assert.Equal(t, "No issues found.\n", output)
	output, err = lintCLI(t, "bundle", "lint", "nested")
	require.ErrorIs(t, err, bundlecmd.ErrLintFailed)
	assert.Contains(t, output, "nested/bundle.uds.hcl:1:")
	_, err = lintCLI(t, "bundle", "lint", filepath.Dir(root))
	require.Error(t, err, "parent must not discover the nested definition")
}

func TestLintCommandMatchesSourceDefinitionPathRules(t *testing.T) {
	workspace := t.TempDir()
	definition := filepath.Join(workspace, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(definition, []byte(`package "broken" {`), 0o600))
	target := lintSource(t)
	require.NoError(t, os.WriteFile(filepath.Join(target, "amd64.yaml"), []byte("valid: true"), 0o600))
	child := filepath.Join(target, "child")
	require.NoError(t, os.Mkdir(child, 0o700))
	link := filepath.Join(workspace, "link")
	require.NoError(t, os.Symlink(child, link))
	parent := link + string(os.PathSeparator) + ".."

	check := func(t *testing.T, path string) {
		t.Helper()
		output, err := lintCLI(t, "bundle", "lint", path, "-a", "amd64")
		require.ErrorIs(t, err, bundlecmd.ErrLintFailed)
		assert.Contains(t, output, filepath.ToSlash(definition)+":1:")
		assert.Contains(t, output, "Unclosed configuration block")

		_, err = executeCLI(t, "", "bundle", "create", "--unsigned", path)
		require.ErrorContains(t, err, "Unclosed configuration block")
		require.ErrorContains(t, err, definition)

		_, err = executeCLI(t, "n\n", "bundle", "dev", "deploy", "--prompt", path)
		require.ErrorContains(t, err, "Unclosed configuration block")
		require.ErrorContains(t, err, definition)
	}
	t.Run("directory", func(t *testing.T) {
		check(t, parent)
	})
	t.Run("definition path", func(t *testing.T) {
		check(t, parent+string(os.PathSeparator)+"bundle.uds.hcl")
	})
	t.Run("relative to a symlinked working directory", func(t *testing.T) {
		t.Chdir(link)
		t.Setenv("PWD", link)
		check(t, filepath.Join("..", "bundle.uds.hcl"))
	})
}

func TestLintCommandMatchesDeploymentConfigPathRules(t *testing.T) {
	workspace := lintSource(t)
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "amd64.yaml"), []byte("host: {{ .vars.domain }}"), 0o600))
	target := t.TempDir()
	child := filepath.Join(target, "child")
	require.NoError(t, os.Mkdir(child, 0o700))
	link := filepath.Join(workspace, "link")
	require.NoError(t, os.Symlink(child, link))
	config := link + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "dev.hcl"

	require.NoError(t, os.WriteFile(filepath.Join(workspace, "domain.txt"), []byte("example.test"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(target, "dev.hcl"), []byte(`variables = { domain = file("domain.txt") }`), 0o600))
	output, err := lintCLI(t, "bundle", "lint", workspace, "-a", "amd64", "--config", config)
	require.NoError(t, err)
	assert.Equal(t, "No issues found.\n", output)
	output, err = executeCLI(t, "n\n", "bundle", "dev", "deploy", "--prompt", workspace, "--config", config)
	require.NoError(t, err)
	assert.Contains(t, output, "deployment cancelled")
}

func TestLintCommandRejectsUnsupportedFlagsAndArguments(t *testing.T) {
	root := lintSource(t)
	for _, args := range [][]string{
		{"bundle", "lint", ""},
		{"bundle", "lint", root, root},
		{"bundle", "lint", root, "--config", ""},
		{"bundle", "lint", root, "--config", filepath.Join(root, "absent.hcl")},
		{"bundle", "lint", root, "--set", "secret-without-equals"},
		{"bundle", "lint", root, "-o", "text"},
		{"bundle", "--output", "text", "lint", root},
		{"bundle", "lint", root, "-o", "json"},
		{"bundle", "lint", root, "-o", "yaml"},
		{"bundle", "--concurrency", "10", "lint", root},
		{"bundle", "lint", root, "--uds-cache", t.TempDir()},
		{"bundle", "lint", root, "--tmp-dir", t.TempDir()},
		{"bundle", "lint", root, "--plain-http"},
		{"bundle", "lint", root, "--skip-tls-verify"},
		{"--prompt", "bundle", "lint", root},
		{"bundle", "lint", root, "--check"},
		{"bundle", "lint", root, "--strict"},
	} {
		t.Run(args[len(args)-1], func(t *testing.T) {
			output, err := lintCLI(t, args...)
			require.Error(t, err)
			assert.Empty(t, output)
		})
	}
}
