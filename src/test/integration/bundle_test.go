// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestCreateAndInspectLocalBundle(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	repository := uniqueRepository(t)
	bundleDir := localBundleFixture(t, workspace, runtime.GOARCH)
	bundlePath := createBundle(t, workspace, bundleDir, repository, runtime.GOARCH)

	inspect := runBundleCLI(t, workspace, "inspect", bundlePath, "--sbom", "--no-color")
	require.NoError(t, inspect.Err, inspect.Stderr)
	require.NotContains(t, inspect.Stdout, "\x1b")
	require.Contains(t, inspect.Stdout+inspect.Stderr, "No SBOMs found in bundle")

	extract := runBundleCLI(t, workspace, "inspect", bundlePath, "--sbom", "--extract", "--no-color")
	require.NoError(t, extract.Err, extract.Stderr)
	require.Contains(t, extract.Stdout+extract.Stderr, "Cannot extract, no SBOMs found in bundle")
}

func TestCreateHonorsNameAndVersionFlags(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	bundleDir := localBundleFixture(t, workspace, runtime.GOARCH)
	name := uniqueRepository(t) + "-name"
	version := "1.2.3"
	outputDir := filepath.Join(workspace, "bundles")

	result := runBundleCLI(t, workspace,
		"create", bundleDir,
		"--confirm", "--insecure", "--no-progress",
		"--architecture", runtime.GOARCH,
		"--name", name,
		"--version", version,
		"--output", outputDir,
	)
	require.NoError(t, result.Err, result.Stderr)
	require.FileExists(t, filepath.Join(outputDir, "uds-bundle-"+name+"-"+runtime.GOARCH+"-"+version+".tar.zst"))
}

func TestCreateRejectsInvalidPackageTimeout(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	result := runBundleCLI(t, workspace,
		"create", testutil.TestDataPath("bundles/22-invalid-timeout"),
		"--confirm", "--insecure", "--architecture", runtime.GOARCH,
	)
	require.Error(t, result.Err)
	require.Contains(t, result.Stdout+result.Stderr, `invalid timeout for package "real-simple": "definitely-not-a-duration"`)
}

func TestCreateRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	bundleDir := localBundleFixture(t, workspace, runtime.GOARCH)
	configPath := filepath.Join(workspace, "uds-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("options:\n  log_levelx: debug\n"), 0o600))

	options := isolatedOptions(t, map[string]string{"UDS_CONFIG": configPath})
	options.Dir = workspace
	result := testutil.RunCLI(t, options,
		"--tmpdir", filepath.Join(workspace, "tmp"),
		"create", bundleDir, "--confirm", "--insecure", "--architecture", runtime.GOARCH,
	)
	require.Error(t, result.Err)
	require.Contains(t, result.Stdout+result.Stderr, "invalid config option: log_levelx")
}

func TestCreateRejectsInvalidBundle(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	bundleDir := filepath.Join(workspace, "bundle")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, config.BundleYAML), []byte(`kind: UDSBundle
metadata:
  name: invalid
  version: 0.0.1
unexpected: value
`), 0o600))

	result := runBundleCLI(t, workspace, "create", bundleDir, "--confirm", "--insecure")
	require.Error(t, result.Err)
	require.Contains(t, result.Stdout+result.Stderr, "unknown field")
}

func createBundle(t *testing.T, workspace, bundleDir, repository, architecture string) string {
	t.Helper()

	outputDir := filepath.Join(workspace, "bundles", architecture)
	require.NoError(t, os.MkdirAll(outputDir, 0o700))
	result := runBundleCLI(t, workspace,
		"create", bundleDir,
		"--confirm",
		"--insecure",
		"--no-progress",
		"--architecture", architecture,
		"--name", repository,
		"--output", outputDir,
	)
	require.NoError(t, result.Err, result.Stderr)

	bundlePath := filepath.Join(outputDir, "uds-bundle-"+repository+"-"+architecture+"-0.0.1.tar.zst")
	require.FileExists(t, bundlePath)
	return bundlePath
}

func localBundleFixture(t *testing.T, workspace, architecture string) string {
	t.Helper()

	bundleDir := testutil.CopyFixture(t, "bundles/24-integration-local")
	packageDir := testutil.CopyFixture(t, "packages/no-cluster/real-simple")
	packageResult := runZarfCLI(t, workspace,
		"zarf", "package", "create", packageDir,
		"--confirm",
		"--architecture", architecture,
		"--output", packageDir,
		"--cache", filepath.Join(workspace, "zarf-cache"),
		"--tmpdir", filepath.Join(workspace, "tmp"),
	)
	require.NoError(t, packageResult.Err, packageResult.Stderr)

	packagePath := filepath.Join(packageDir, "zarf-package-real-simple-"+architecture+"-0.0.1.tar.zst")
	require.FileExists(t, packagePath)

	bundlePath := filepath.Join(bundleDir, config.BundleYAML)
	contents, err := os.ReadFile(bundlePath)
	require.NoError(t, err)
	updated := strings.Replace(string(contents), "__LOCAL_PACKAGE_PATH__", filepath.ToSlash(packagePath), 1)
	require.NotEqual(t, string(contents), updated)
	require.NoError(t, os.WriteFile(bundlePath, []byte(updated), 0o600))
	return bundleDir
}

func runBundleCLI(t *testing.T, workspace string, args ...string) testutil.CommandResult {
	t.Helper()

	command := append([]string{"--tmpdir", filepath.Join(workspace, "tmp")}, args...)
	options := isolatedOptions(t, nil)
	options.Dir = workspace
	return testutil.RunCLI(t, options, command...)
}

func runZarfCLI(t *testing.T, workspace string, args ...string) testutil.CommandResult {
	t.Helper()

	options := isolatedOptions(t, nil)
	options.Dir = workspace
	return testutil.RunCLI(t, options, args...)
}

func uniqueRepository(t *testing.T) string {
	t.Helper()

	name := strings.ToLower(t.Name())
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "-")
	return strings.Trim(name, "-")
}
