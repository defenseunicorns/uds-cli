// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package integration

import (
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestBundleCommandVariables(t *testing.T) {
	t.Parallel()

	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/02-variables", "no-cluster")
	workspace := t.TempDir()
	for _, relative := range []string{"../../packages/no-cluster/output-var", "../../packages/no-cluster/receive-var"} {
		packageDir := filepath.Clean(filepath.Join(bundleDir, relative))
		testutil.CreatePackageForTest(t, packageDir, packageDir)
	}
	opts := isolatedOptions(t, map[string]string{
		"UDS_ANIMAL": "Unicorns",
		"UDS_CONFIG": filepath.Join(bundleDir, "uds-config.yaml"),
	})
	opts.Dir = workspace
	bundlePath := createCommandVariableBundle(t, opts, bundleDir)

	result := runVariableCLI(t, opts, "deploy", bundlePath, "--retries", "1", "--confirm", "--no-color")
	require.NoError(t, result.Err, result.Stderr)
	require.Contains(t, result.Stdout, `SENSITIVE_VAR: "****"`)
	require.NotContains(t, result.Stdout, "e2e-sensitive-zarf-value")
	require.Contains(t, result.Stderr, "This fun-fact was imported: Unicorns are the national animal of Scotland")
	require.Contains(t, result.Stderr, "This fun-fact demonstrates precedence: The Red Dragon is the national symbol of Wales")
	require.Contains(t, result.Stderr, "shared var in output-var pkg: burning.boats")
	require.Contains(t, result.Stderr, "shared var in receive-var pkg: burning.boats")

	setResult := runVariableCLI(t, opts, "deploy", bundlePath, "--confirm", "--set", "ANIMAL=Longhorns", "--set", "COUNTRY=Texas")
	require.NoError(t, setResult.Err, setResult.Stderr)
	require.Contains(t, setResult.Stderr, "This fun-fact was imported: Longhorns are the national animal of Texas")
	require.NotContains(t, setResult.Stderr, "This fun-fact was imported: Unicorns are the national animal of Scotland")

	packageSet := runVariableCLI(t, opts, "deploy", bundlePath, "--confirm", "--set", "output-var.SPECIFIC_PKG_VAR=output", "--set", "receive-var.SPECIFIC_PKG_VAR=receive")
	require.NoError(t, packageSet.Err, packageSet.Stderr)
	require.Contains(t, packageSet.Stderr, "output-var SPECIFIC_PKG_VAR = output")
	require.Contains(t, packageSet.Stderr, "receive-var SPECIFIC_PKG_VAR = receive")
}

func TestBundleVariableValidation(t *testing.T) {
	t.Parallel()

	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/02-variables/bad-var-name", "no-cluster")
	for _, relative := range []string{"../../../packages/no-cluster/output-var", "../../../packages/no-cluster/receive-var"} {
		packageDir := filepath.Clean(filepath.Join(bundleDir, relative))
		testutil.CreatePackageForTest(t, packageDir, packageDir)
	}
	opts := isolatedOptions(t, nil)
	opts.Dir = t.TempDir()
	result := runVariableCLI(t, opts, "create", bundleDir, "--confirm", "--insecure", "--output", opts.Dir)
	require.Error(t, result.Err)
	require.Contains(t, result.Stderr, "does not have a matching export")
}

func TestBundleVariableExportNameCollision(t *testing.T) {
	t.Parallel()

	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/02-variables/export-name-collision", "no-cluster")
	for _, relative := range []string{
		"../../../packages/no-cluster/output-var",
		"../../../packages/no-cluster/output-var-collision",
		"../../../packages/no-cluster/receive-var",
	} {
		packageDir := filepath.Clean(filepath.Join(bundleDir, relative))
		testutil.CreatePackageForTest(t, packageDir, packageDir)
	}
	opts := isolatedOptions(t, map[string]string{"UDS_ANIMAL": "Unicorns"})
	opts.Dir = t.TempDir()
	bundlePath := createCommandVariableBundle(t, opts, bundleDir)
	result := runVariableCLI(t, opts, "deploy", bundlePath, "--confirm", "--retries", "1")
	require.NoError(t, result.Err, result.Stderr)
	output := result.Stdout + result.Stderr
	require.Contains(t, output, "This fun-fact was imported: Daffodils are the national flower of Wales")
	require.NotContains(t, output, "This fun-fact was imported: Unicorns are the national animal of Scotland")
}

func TestInspectListsBundleVariables(t *testing.T) {
	t.Parallel()

	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/02-variables", "no-cluster")
	for _, relative := range []string{"../../packages/no-cluster/output-var", "../../packages/no-cluster/receive-var"} {
		packageDir := filepath.Clean(filepath.Join(bundleDir, relative))
		testutil.CreatePackageForTest(t, packageDir, packageDir)
	}

	opts := isolatedOptions(t, map[string]string{"UDS_ANIMAL": "Unicorns"})
	opts.Dir = t.TempDir()
	bundlePath := createCommandVariableBundle(t, opts, bundleDir)

	noColor := runVariableCLI(t, opts, "inspect", bundlePath, "--list-variables", "--no-color")
	require.NoError(t, noColor.Err, noColor.Stderr)
	require.NotContains(t, noColor.Stdout, "\x1b")
	require.Contains(t, noColor.Stdout, "output-var:")
	require.Contains(t, noColor.Stdout, "ANIMAL")

	withColor := runVariableCLI(t, opts, "inspect", bundlePath, "--list-variables")
	require.NoError(t, withColor.Err, withColor.Stderr)
	require.Contains(t, withColor.Stdout, "\x1b")

	registry := testutil.NewRegistry(t)
	published := runVariableCLI(t, opts, "publish", bundlePath, registry.Host, "--insecure")
	require.NoError(t, published.Err, published.Stderr)
	remoteRef := "oci://" + registry.Host + "/variables:0.0.1"
	remote := runVariableCLI(t, opts, "inspect", remoteRef, "--list-variables", "--insecure", "--no-color")
	require.NoError(t, remote.Err, remote.Stderr)
	require.Contains(t, remote.Stdout, "output-var:")

	fromYAML := runVariableCLI(t, opts, "inspect", filepath.Join(bundleDir, "uds-bundle.yaml"), "--list-variables", "--no-color")
	require.NoError(t, fromYAML.Err, fromYAML.Stderr)
	require.Contains(t, fromYAML.Stdout, "output-var:")
}

func createCommandVariableBundle(t *testing.T, opts testutil.CommandOptions, bundleDir string) string {
	t.Helper()
	result := runVariableCLI(t, opts, "create", bundleDir, "--confirm", "--insecure", "--output", opts.Dir)
	require.NoError(t, result.Err, result.Stderr)
	archives, err := filepath.Glob(filepath.Join(opts.Dir, "uds-bundle-*.tar.zst"))
	require.NoError(t, err)
	require.Len(t, archives, 1)
	return archives[0]
}

func runVariableCLI(t *testing.T, opts testutil.CommandOptions, args ...string) testutil.CommandResult {
	t.Helper()
	args = append([]string{"--tmpdir", filepath.Join(opts.Dir, "tmp"), "--uds-cache", filepath.Join(opts.Dir, "cache"), "--no-progress"}, args...)
	return testutil.RunCLI(t, opts, args...)
}
