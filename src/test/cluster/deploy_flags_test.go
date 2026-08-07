// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
)

const deployFlagsTimeout = 5 * time.Minute

var clusterPackageArchives = struct {
	sync.Mutex
	paths map[string]string
}{paths: map[string]string{}} //nolint:gochecknoglobals // Shared immutable package archives are scoped to the test suite.

func TestDeployPackagesFlag(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := prepareLocalAndRemoteBundle(t, opts, namespace)
	bundlePath := createClusterBundle(t, opts, bundleDir, "../../packages/podinfo")

	t.Run("local package", func(t *testing.T) {
		result := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--packages", "podinfo")
		require.NoError(t, result.Err, result.Stderr)
		k8s.WaitForDeploymentReady(namespace, "podinfo", deployFlagsTimeout)
		k8s.AssertDeploymentDoesNotExist(namespace, "nginx-deployment")
		remove := runClusterCLI(t, opts, "remove", bundlePath, "--confirm", "--insecure", "--packages", "podinfo")
		require.NoError(t, remove.Err, remove.Stderr)
		k8s.AssertDeploymentDoesNotExist(namespace, "podinfo")
	})

	t.Run("remote package", func(t *testing.T) {
		result := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--packages", "nginx")
		require.NoError(t, result.Err, result.Stderr)
		k8s.WaitForDeploymentReady(namespace, "nginx-deployment", deployFlagsTimeout)
		k8s.AssertDeploymentDoesNotExist(namespace, "podinfo")
		remove := runClusterCLI(t, opts, "remove", bundlePath, "--confirm", "--insecure", "--packages", "nginx")
		require.NoError(t, remove.Err, remove.Stderr)
		k8s.AssertDeploymentDoesNotExist(namespace, "nginx-deployment")
	})

	t.Run("multiple packages", func(t *testing.T) {
		result := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--packages", "podinfo,nginx")
		require.NoError(t, result.Err, result.Stderr)
		k8s.WaitForDeploymentReady(namespace, "podinfo", deployFlagsTimeout)
		k8s.WaitForDeploymentReady(namespace, "nginx-deployment", deployFlagsTimeout)
		remove := runClusterCLI(t, opts, "remove", bundlePath, "--confirm", "--insecure", "--packages", "podinfo,nginx")
		require.NoError(t, remove.Err, remove.Stderr)
		k8s.AssertDeploymentDoesNotExist(namespace, "podinfo")
		k8s.AssertDeploymentDoesNotExist(namespace, "nginx-deployment")
	})

	for _, operation := range []string{"deploy", "remove"} {
		operation := operation
		t.Run("invalid package "+operation, func(t *testing.T) {
			result := runClusterCLI(t, opts, operation, bundlePath, "--confirm", "--insecure", "--packages", "podinfo,nginx,peanuts")
			require.Error(t, result.Err)
			require.Contains(t, result.Stderr, "invalid zarf packages specified by --packages")
		})
	}
}

func TestDeployResumeFlag(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := prepareLocalAndRemoteBundle(t, opts, namespace)
	bundlePath := createClusterBundle(t, opts, bundleDir, "../../packages/podinfo")

	first := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--packages", "podinfo")
	require.NoError(t, first.Err, first.Stderr)
	k8s.WaitForDeploymentReady(namespace, "podinfo", deployFlagsTimeout)
	k8s.AssertDeploymentDoesNotExist(namespace, "nginx-deployment")

	resumed := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--resume")
	require.NoError(t, resumed.Err, resumed.Stderr)
	k8s.WaitForDeploymentReady(namespace, "nginx-deployment", deployFlagsTimeout)

	removePodinfo := runClusterCLI(t, opts, "remove", bundlePath, "--confirm", "--insecure", "--packages", "podinfo")
	require.NoError(t, removePodinfo.Err, removePodinfo.Stderr)
	k8s.AssertDeploymentDoesNotExist(namespace, "podinfo")
	k8s.AssertDeploymentExists(namespace, "nginx-deployment")

	resumed = runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--resume")
	require.NoError(t, resumed.Err, resumed.Stderr)
	k8s.WaitForDeploymentReady(namespace, "podinfo", deployFlagsTimeout)

	removed := runClusterCLI(t, opts, "remove", bundlePath, "--confirm", "--insecure")
	require.NoError(t, removed.Err, removed.Stderr)
	k8s.AssertDeploymentDoesNotExist(namespace, "podinfo")
	k8s.AssertDeploymentDoesNotExist(namespace, "nginx-deployment")
}

func TestResumeFlagWithPackageNamespaceOverrideDuplicates(t *testing.T) {
	t.Parallel()

	namespaceA, k8s := testutil.AllocateNamespace(t, suite)
	namespaceB, _ := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	seedDir := testutil.PrepareBundleForNamespace(t, "bundles/07-helm-overrides/package-namespace-resume-seed", namespaceA)
	seedPath := createClusterBundle(t, opts, seedDir, "../../../packages/nginx-namespace-override")
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/07-helm-overrides/package-namespace-resume", namespaceA)
	setBundlePackageNamespaces(t, bundleDir, []string{namespaceA, namespaceB})
	bundlePath := createClusterBundle(t, opts, bundleDir, "../../../packages/nginx-namespace-override")
	defer removeBundle(t, opts, bundlePath, nil)
	defer removeBundle(t, opts, seedPath, nil)

	seed := runClusterCLI(t, opts, "deploy", seedPath, "--confirm")
	require.NoError(t, seed.Err, seed.Stderr)
	k8s.WaitForDeploymentReady(namespaceA, "nginx-deployment", deployFlagsTimeout)

	resume := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--resume")
	require.NoError(t, resume.Err, resume.Stderr)
	k8s.WaitForDeploymentReady(namespaceB, "nginx-deployment", deployFlagsTimeout)

	removed := runClusterCLI(t, opts, "remove", bundlePath, "--confirm", "--insecure")
	require.NoError(t, removed.Err, removed.Stderr)
	k8s.AssertDeploymentDoesNotExist(namespaceA, "nginx-deployment")
	k8s.AssertDeploymentDoesNotExist(namespaceB, "nginx-deployment")
}

func createClusterBundle(t *testing.T, opts testutil.CommandOptions, bundleDir string, packagePaths ...string) string {
	t.Helper()
	for _, packagePath := range packagePaths {
		path := filepath.Clean(filepath.Join(bundleDir, packagePath))
		archive := createClusterPackage(t, opts, path, path)
		setBundlePackageArchive(t, bundleDir, path, archive)
	}
	result := runClusterCLI(t, opts, "create", bundleDir, "--confirm", "--insecure", "--output", bundleDir)
	require.NoError(t, result.Err, result.Stderr)
	path := localBundleArchive(bundleDir)
	require.FileExists(t, path)
	return path
}

// setBundlePackageArchive makes the bundle consume the exact archive created
// above. This is required when a test gives a Zarf package a unique metadata
// name: the archive filename no longer matches the bundle package name.
func setBundlePackageArchive(t *testing.T, bundleDir, packageDir, archive string) {
	t.Helper()

	bundlePath := filepath.Join(bundleDir, config.BundleYAML)
	var bundle types.UDSBundle
	require.NoError(t, utils.ReadYAMLStrict(bundlePath, &bundle))

	relativeArchive, err := filepath.Rel(bundleDir, archive)
	require.NoError(t, err)
	updated := false
	for index := range bundle.Packages {
		pkg := &bundle.Packages[index]
		if filepath.Clean(filepath.Join(bundleDir, pkg.Path)) == packageDir {
			pkg.Path = relativeArchive
			updated = true
		}
	}
	require.True(t, updated, "package directory %q not found in bundle", packageDir)
	require.NoError(t, zarfUtils.WriteYaml(bundlePath, &bundle, 0o600))
}

func createClusterPackage(t *testing.T, opts testutil.CommandOptions, source, outputDir string) string {
	t.Helper()
	if key := cachedClusterPackageKey(source); key != "" {
		return cachedClusterPackage(t, opts, key, source)
	}
	return createClusterPackageArchive(t, opts, source, outputDir)
}

func cachedClusterPackage(t *testing.T, opts testutil.CommandOptions, key, source string) string {
	t.Helper()
	clusterPackageArchives.Lock()
	defer clusterPackageArchives.Unlock()

	if archive := clusterPackageArchives.paths[key]; archive != "" {
		return archive
	}
	archive := createClusterPackageArchive(t, opts, source, filepath.Join(suite.WorkspacePath, "package-archives", key))
	clusterPackageArchives.paths[key] = archive
	return archive
}

func createClusterPackageArchive(t *testing.T, opts testutil.CommandOptions, source, outputDir string) string {
	t.Helper()
	result := runClusterCLI(t, opts, "zarf", "package", "create", source,
		"--confirm", "--skip-sbom", "--output", outputDir,
		"--tmpdir", filepath.Join(outputDir, "tmp"), "--cache", filepath.Join(outputDir, "cache"))
	require.NoError(t, result.Err, result.Stderr)
	archives, err := filepath.Glob(filepath.Join(outputDir, "zarf-package-*.tar.zst"))
	require.NoError(t, err)
	require.Len(t, archives, 1, "expected one package archive in %q", outputDir)
	return archives[0]
}

func cachedClusterPackageKey(source string) string {
	path := filepath.ToSlash(filepath.Clean(source))
	for suffix, key := range map[string]string{
		"/packages/nginx/refs":               "nginx-refs",
		"/packages/nginx":                    "nginx",
		"/packages/nginx-namespace-override": "nginx-namespace-override",
		"/packages/podinfo":                  "podinfo",
	} {
		if strings.HasSuffix(path, suffix) {
			return key
		}
	}
	return ""
}

func setBundlePackageNamespaces(t *testing.T, bundleDir string, namespaces []string) {
	t.Helper()
	path := filepath.Join(bundleDir, config.BundleYAML)
	var bundle types.UDSBundle
	require.NoError(t, utils.ReadYAMLStrict(path, &bundle))
	require.Len(t, bundle.Packages, len(namespaces))
	for index := range bundle.Packages {
		bundle.Packages[index].Namespace = namespaces[index]
	}
	require.NoError(t, zarfUtils.WriteYaml(path, &bundle, 0o600))
}
