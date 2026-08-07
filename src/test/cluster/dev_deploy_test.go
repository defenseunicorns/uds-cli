// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
)

const devDeployTimeout = 5 * time.Minute

func TestDevDeployLocalAndRemotePackages(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := prepareLocalAndRemoteBundle(t, opts, namespace)
	defer removeBundle(t, opts, localBundleArchive(bundleDir), nil)

	result := runClusterCLI(t, opts, "dev", "deploy", bundleDir, "--insecure")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "podinfo", devDeployTimeout)
	k8s.WaitForDeploymentReady(namespace, "nginx-deployment", devDeployTimeout)
}

func TestDevDeployPackageSelection(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := prepareLocalBundle(t, namespace, "packages/nginx")
	defer removeBundle(t, opts, localBundleArchive(bundleDir), []string{"podinfo"})

	result := runClusterCLI(t, opts, "dev", "deploy", bundleDir, "--packages", "podinfo", "--insecure")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "podinfo", devDeployTimeout)
	k8s.AssertDeploymentDoesNotExist(namespace, "nginx-deployment")
}

func TestDevDeployReferenceOverride(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := prepareLocalBundle(t, namespace, "packages/nginx/refs")
	defer removeBundle(t, opts, localBundleArchive(bundleDir), nil)

	result := runClusterCLI(t, opts, "dev", "deploy", bundleDir, "--ref", "nginx=0.0.2", "--insecure")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "nginx-deployment", devDeployTimeout)
	require.Contains(t, k8s.DeploymentImage(namespace, "nginx-deployment"), "nginx:1.26.0")
}

func TestDevDeployPackageFlavor(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/15-dev-deploy", namespace)
	defer removeBundle(t, opts, localBundleArchive(bundleDir), nil)

	result := runClusterCLI(t, opts, "dev", "deploy", bundleDir, "--flavor", "podinfo=patchVersion3")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "podinfo", devDeployTimeout)
	require.Contains(t, k8s.DeploymentImage(namespace, "podinfo"), "podinfo:6.6.3")
}

func TestDevDeployGlobalFlavor(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/15-dev-deploy", namespace)
	defer removeBundle(t, opts, localBundleArchive(bundleDir), nil)

	result := runClusterCLI(t, opts, "dev", "deploy", bundleDir, "--flavor", "patchVersion3", "--force-create")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "podinfo", devDeployTimeout)
	require.Contains(t, k8s.DeploymentImage(namespace, "podinfo"), "podinfo:6.6.3")
}

func TestDevDeployForceCreatesRequestedFlavor(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/15-dev-deploy", namespace)
	packageDir := filepath.Clean(filepath.Join(bundleDir, "../../packages/podinfo/flavors"))
	created := runClusterCLI(t, opts, "zarf", "package", "create", packageDir,
		"--flavor", "patchVersion3", "--confirm", "--skip-sbom", "--output", packageDir,
		"--tmpdir", filepath.Join(opts.Dir, "zarf-tmp"), "--cache", filepath.Join(opts.Dir, "zarf-cache"))
	require.NoError(t, created.Err, created.Stderr)
	defer removeBundle(t, opts, localBundleArchive(bundleDir), nil)

	result := runClusterCLI(t, opts, "dev", "deploy", bundleDir,
		"--flavor", "podinfo=patchVersion2", "--force-create")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "podinfo", devDeployTimeout)
	require.Contains(t, k8s.DeploymentImage(namespace, "podinfo"), "podinfo:6.6.2")
}

func TestDevDeployRemoteBundleFromLocalRegistry(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	registry := testutil.NewRegistry(t)
	bundleDir := prepareLocalAndRemoteBundle(t, opts, namespace)
	localPackageDir := filepath.Clean(filepath.Join(bundleDir, "../../packages/podinfo"))
	testutil.CreatePackageForTest(t, localPackageDir, localPackageDir)
	outputDir := t.TempDir()
	created := runClusterCLI(t, opts, "create", bundleDir, "--confirm", "--insecure", "--output", outputDir)
	require.NoError(t, created.Err, created.Stderr)
	bundleArchive := filepath.Join(outputDir, filepath.Base(localBundleArchive(bundleDir)))
	require.FileExists(t, bundleArchive)
	remoteRef := "oci://" + registry.Host + "/dev-deploy/test-local-and-remote:0.0.1"
	published := runClusterCLI(t, opts, "publish", bundleArchive, registry.Host+"/dev-deploy", "--insecure")
	require.NoError(t, published.Err, published.Stderr)
	defer removeBundle(t, opts, remoteRef, nil)

	result := runClusterCLI(t, opts, "dev", "deploy", remoteRef, "--insecure")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "podinfo", devDeployTimeout)
	k8s.WaitForDeploymentReady(namespace, "nginx-deployment", devDeployTimeout)
}

func TestDevDeploySetVariables(t *testing.T) {
	t.Parallel()

	namespace, _ := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/02-variables", namespace)
	defer removeBundle(t, opts, localBundleArchive(bundleDir), nil)

	result := runClusterCLI(t, opts, "dev", "deploy", bundleDir,
		"--set", "ANIMAL=Longhorns", "--set", "COUNTRY=Texas", "--log-level", "debug")
	require.NoError(t, result.Err, result.Stderr)
	require.Contains(t, result.Stderr, "This fun-fact was imported: Longhorns are the national animal of Texas")
	require.NotContains(t, result.Stderr, "This fun-fact was imported: Unicorns are the national animal of Scotland")
}

func isolatedClusterOptions(t *testing.T) testutil.CommandOptions {
	t.Helper()
	workspace := t.TempDir()
	helmHome := filepath.Join(workspace, "helm")
	opts := suite.CommandOptions()
	opts.Dir = workspace
	opts.Env["HELM_CACHE_HOME"] = filepath.Join(helmHome, "cache")
	opts.Env["HELM_CONFIG_HOME"] = filepath.Join(helmHome, "config")
	opts.Env["HELM_DATA_HOME"] = filepath.Join(helmHome, "data")
	return opts
}

func runClusterCLI(t *testing.T, opts testutil.CommandOptions, args ...string) testutil.CommandResult {
	t.Helper()
	if len(args) > 0 && args[0] == "zarf" {
		return testutil.RunCLI(t, opts, args...)
	}
	args = append([]string{
		"--tmpdir", filepath.Join(opts.Dir, "tmp"),
		"--uds-cache", filepath.Join(opts.Dir, "cache"),
		"--no-progress",
	}, args...)
	return testutil.RunCLI(t, opts, args...)
}

func prepareLocalAndRemoteBundle(t *testing.T, opts testutil.CommandOptions, namespace string) string {
	t.Helper()
	registry := testutil.NewRegistry(t)
	repository := registry.Host + "/dev-deploy/nginx"
	packageArchive := testutil.CreatePackageForTest(t, testutil.CopyFixture(t, "packages/nginx"), t.TempDir())
	publishPackage(t, opts, packageArchive, registry.Host+"/dev-deploy")
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/03-local-and-remote", namespace)
	setPackageRepository(t, bundleDir, "nginx", repository)
	return bundleDir
}

// prepareLocalBundle replaces every package source with a test-private archive.
// This keeps tests that exercise local dev-deploy behavior independent of the
// registry used by the remote-bundle test.
func prepareLocalBundle(t *testing.T, namespace, nginxFixture string) string {
	t.Helper()

	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/03-local-and-remote", namespace)
	podinfoDir := filepath.Clean(filepath.Join(bundleDir, "../../packages/podinfo"))
	nginxDir := testutil.CopyFixture(t, nginxFixture)
	setPackagePath(t, bundleDir, "podinfo", testutil.CreatePackageForTest(t, podinfoDir, t.TempDir()))
	setPackagePath(t, bundleDir, "nginx", testutil.CreatePackageForTest(t, nginxDir, t.TempDir()))
	return bundleDir
}

func publishPackage(t *testing.T, opts testutil.CommandOptions, archive, repository string) {
	t.Helper()
	result := runClusterCLI(t, opts, "zarf", "package", "publish", archive,
		"oci://"+repository, "--plain-http", "--tmpdir", filepath.Join(opts.Dir, "zarf-tmp"),
		"--cache", filepath.Join(opts.Dir, "zarf-cache"))
	require.NoError(t, result.Err, result.Stderr)
}

func setPackageRepository(t *testing.T, bundleDir, packageName, repository string) {
	t.Helper()
	bundlePath := filepath.Join(bundleDir, config.BundleYAML)
	var bundle types.UDSBundle
	require.NoError(t, utils.ReadYAMLStrict(bundlePath, &bundle))
	updated := false
	for index := range bundle.Packages {
		if bundle.Packages[index].Name == packageName {
			bundle.Packages[index].Repository = repository
			updated = true
		}
	}
	require.True(t, updated, "package %q not found in bundle", packageName)
	require.NoError(t, zarfUtils.WriteYaml(bundlePath, &bundle, 0o600))
}

func setPackagePath(t *testing.T, bundleDir, packageName, path string) {
	t.Helper()
	bundlePath := filepath.Join(bundleDir, config.BundleYAML)
	var bundle types.UDSBundle
	require.NoError(t, utils.ReadYAMLStrict(bundlePath, &bundle))
	updated := false
	for index := range bundle.Packages {
		if bundle.Packages[index].Name == packageName {
			bundle.Packages[index].Repository = ""
			bundle.Packages[index].Path = path
			updated = true
		}
	}
	require.True(t, updated, "package %q not found in bundle", packageName)
	require.NoError(t, zarfUtils.WriteYaml(bundlePath, &bundle, 0o600))
}

func localBundleArchive(bundleDir string) string {
	var bundle types.UDSBundle
	if err := utils.ReadYAMLStrict(filepath.Join(bundleDir, config.BundleYAML), &bundle); err != nil {
		panic(err)
	}
	return filepath.Join(bundleDir, "uds-bundle-"+bundle.Metadata.Name+"-"+runtime.GOARCH+"-"+bundle.Metadata.Version+".tar.zst")
}

func removeBundle(t *testing.T, opts testutil.CommandOptions, source string, packages []string) {
	t.Helper()
	if filepath.IsAbs(source) {
		if _, err := os.Stat(source); err != nil {
			return
		}
	}
	args := []string{"remove", source, "--confirm", "--insecure"}
	if len(packages) > 0 {
		args = append(args, "--packages", packages[0])
	}
	result := runClusterCLI(t, opts, args...)
	if result.Err != nil {
		t.Errorf("remove test bundle %q: %v\n%s", source, result.Err, result.Stderr)
	}
}
