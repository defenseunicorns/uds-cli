// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
)

func TestLogsAfterBundleCreate(t *testing.T) {
	t.Parallel()

	opts := isolatedClusterOptions(t)
	prepareRealSimpleBundle(t, opts)

	logs := runClusterCLI(t, opts, "logs")
	require.NoError(t, logs.Err, logs.Stderr)
	require.Contains(t, logs.Stdout, "DEBUG")
}

func TestSimpleBundleRunsZarfAction(t *testing.T) {
	t.Parallel()

	opts := isolatedClusterOptions(t)
	bundlePath := prepareRealSimpleBundle(t, opts)
	defer removeBundle(t, opts, bundlePath, nil)

	deployed := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--retries", "1")
	require.NoError(t, deployed.Err, deployed.Stderr)
	require.Contains(t, deployed.Stdout+deployed.Stderr, "pulling package")
}

func TestBundleYAMLFileLifecycle(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.CopyFixture(t, "bundles/09-uds-bundle-yml")
	packageDir := testutil.CopyFixture(t, "packages/nginx")
	packageArchive := createClusterPackage(t, opts, packageDir, filepath.Join(opts.Dir, "packages"))

	bundleConfig := filepath.Join(bundleDir, "uds-bundle.yml")
	var bundle types.UDSBundle
	require.NoError(t, utils.ReadYAMLStrict(bundleConfig, &bundle))
	require.Len(t, bundle.Packages, 1)
	bundle.Packages[0].Path = packageArchive
	bundle.Packages[0].Namespace = namespace
	require.NoError(t, zarfUtils.WriteYaml(bundleConfig, &bundle, 0o600))

	created := runClusterCLI(t, opts, "create", bundleDir, "--confirm", "--insecure", "--output", bundleDir)
	require.NoError(t, created.Err, created.Stderr)
	bundlePath := filepath.Join(bundleDir, "uds-bundle-yml-example-"+runtime.GOARCH+"-0.0.1.tar.zst")
	require.FileExists(t, bundlePath)

	configPath := filepath.Join(bundleDir, "uds-config.yml")
	deployed := runClusterCLIWithEnv(t, opts, map[string]string{"UDS_CONFIG": configPath}, "deploy", bundlePath, "--confirm", "--retries", "1")
	require.NoError(t, deployed.Err, deployed.Stderr)
	k8s.WaitForDeploymentReady(namespace, "nginx-deployment", deployFlagsTimeout)

	removed := runClusterCLI(t, opts, "remove", bundlePath, "--confirm", "--insecure")
	require.NoError(t, removed.Err, removed.Stderr)
	k8s.AssertDeploymentDoesNotExist(namespace, "nginx-deployment")
}

func TestBundleCommandsUseConfiguredTemporaryDirectory(t *testing.T) {
	t.Parallel()

	opts := isolatedClusterOptions(t)
	temporaryDirectory := filepath.Join(opts.Dir, "custom-tmp")
	require.NoError(t, os.MkdirAll(temporaryDirectory, 0o700))
	bundleDir := testutil.CopyFixture(t, "bundles/11-real-simple")
	packageDir := testutil.CopyFixture(t, "packages/no-cluster/real-simple")
	packageArchive := createClusterPackage(t, opts, packageDir, filepath.Join(opts.Dir, "packages"))
	setPackagePath(t, bundleDir, "real-simple", packageArchive)
	bundlePath := filepath.Join(bundleDir, "uds-bundle-real-simple-"+runtime.GOARCH+"-0.0.1.tar.zst")

	for _, command := range [][]string{
		{"create", bundleDir, "--confirm", "--insecure", "--output", bundleDir},
		{"deploy", bundlePath, "--confirm", "--retries", "1"},
		{"remove", bundlePath, "--confirm", "--insecure"},
	} {
		args := append([]string{"--tmpdir", temporaryDirectory, "--uds-cache", filepath.Join(opts.Dir, "cache"), "--no-progress"}, command...)
		result := testutil.RunCLI(t, opts, args...)
		require.NoError(t, result.Err, result.Stderr)
	}

	entries, err := os.ReadDir(temporaryDirectory)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "create, deploy, and remove should use the configured temporary directory")
}

func TestDeployRejectsBundleForDifferentArchitecture(t *testing.T) {
	t.Parallel()

	architecture := "amd64"
	if runtime.GOARCH == architecture {
		architecture = "arm64"
	}
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.CopyFixture(t, "bundles/11-real-simple")
	packageDir := testutil.CopyFixture(t, "packages/no-cluster/real-simple")
	packageOutput := filepath.Join(opts.Dir, "packages", architecture)
	createdPackage := runClusterCLI(t, opts, "zarf", "package", "create", packageDir,
		"--confirm", "--skip-sbom", "--architecture", architecture, "--output", packageOutput,
		"--tmpdir", filepath.Join(packageOutput, "tmp"), "--cache", filepath.Join(packageOutput, "cache"))
	require.NoError(t, createdPackage.Err, createdPackage.Stderr)
	archives, err := filepath.Glob(filepath.Join(packageOutput, "zarf-package-*.tar.zst"))
	require.NoError(t, err)
	require.Len(t, archives, 1)
	setPackagePath(t, bundleDir, "real-simple", archives[0])

	createdBundle := runClusterCLI(t, opts, "create", bundleDir, "--confirm", "--insecure", "--architecture", architecture, "--output", bundleDir)
	require.NoError(t, createdBundle.Err, createdBundle.Stderr)
	bundlePath := filepath.Join(bundleDir, "uds-bundle-real-simple-"+architecture+"-0.0.1.tar.zst")

	deployed := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm")
	require.Error(t, deployed.Err)
	require.Contains(t, deployed.Stdout+deployed.Stderr, fmt.Sprintf("arch %s does not match cluster arch, [%s]", architecture, runtime.GOARCH))
}

func TestBundleComponentNameContainingAuthDeploys(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/16-zarf-bug", namespace)
	packageRoot := testutil.CopyFixture(t, "packages")
	packageDir := filepath.Join(packageRoot, "zarf-bug")
	chartDir := filepath.Join(packageRoot, "helm", "unicorn-podinfo")
	added := runClusterCLI(t, opts, "zarf", "tools", "helm", "repo", "add", "podinfo", "https://stefanprodan.github.io/podinfo")
	require.NoError(t, added.Err, added.Stderr)
	updated := runClusterCLI(t, opts, "zarf", "tools", "helm", "dependency", "update", chartDir)
	require.NoError(t, updated.Err, updated.Stderr)
	packageArchive := createClusterPackage(t, opts, packageDir, filepath.Join(opts.Dir, "packages"))
	setPackagePath(t, bundleDir, "zarf-component-name-bug", packageArchive)

	created := runClusterCLI(t, opts, "create", bundleDir, "--confirm", "--insecure", "--output", bundleDir)
	require.NoError(t, created.Err, created.Stderr)
	bundlePath := localBundleArchive(bundleDir)

	deployed := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--insecure", "--retries", "1")
	require.NoError(t, deployed.Err, deployed.Stderr)
	require.NotContains(t, deployed.Stdout+deployed.Stderr, `unable to deploy component "authservice": unable to decompress`)
	k8s.WaitForDeploymentReady(namespace, "unicorn-podinfo", deployFlagsTimeout)

	removed := runClusterCLI(t, opts, "remove", bundlePath, "--confirm", "--insecure")
	require.NoError(t, removed.Err, removed.Stderr)
	k8s.AssertDeploymentDoesNotExist(namespace, "unicorn-podinfo")
}

func prepareRealSimpleBundle(t *testing.T, opts testutil.CommandOptions) string {
	t.Helper()

	bundleDir := testutil.CopyFixture(t, "bundles/11-real-simple")
	packageDir := testutil.CopyFixture(t, "packages/no-cluster/real-simple")
	packageArchive := createClusterPackage(t, opts, packageDir, filepath.Join(opts.Dir, "packages"))
	setPackagePath(t, bundleDir, "real-simple", packageArchive)

	created := runClusterCLI(t, opts, "create", bundleDir, "--confirm", "--insecure", "--output", bundleDir)
	require.NoError(t, created.Err, created.Stderr)
	bundlePath := localBundleArchive(bundleDir)
	require.FileExists(t, bundlePath)
	return bundlePath
}

func runClusterCLIWithEnv(t *testing.T, opts testutil.CommandOptions, env map[string]string, args ...string) testutil.CommandResult {
	t.Helper()

	options := opts
	options.Env = make(map[string]string, len(opts.Env)+len(env))
	for key, value := range opts.Env {
		options.Env[key] = value
	}
	for key, value := range env {
		options.Env[key] = value
	}
	return testutil.RunCLI(t, options, append([]string{"--tmpdir", filepath.Join(opts.Dir, "tmp"), "--uds-cache", filepath.Join(opts.Dir, "cache"), "--no-progress"}, args...)...)
}
