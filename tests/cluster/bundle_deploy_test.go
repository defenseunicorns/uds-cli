// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

const podinfoReadyTimeout = 5 * time.Minute

func TestDeployVariablesBundleWithPodinfo(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateTestNamespace(t, sharedClusterName, namespaceCleanupTimeout)
	bundleDir := testutil.PreparePodinfoBundle(t, testEnv.podinfoPackagePath, "podinfo", namespace)
	testutil.RegisterBundleCleanup(t, bundleDir, namespaceCleanupTimeout)

	testutil.RequireCLI(t,
		"bundle", "dev", "deploy", bundleDir,
		"--config", testutil.TestDataPath("bundles/deploy/variables/full-config.uds.hcl"),
	)

	assertPodinfoConfiguration(t, k8s, namespace)
}

func TestDeployFromArtifactAndRemove(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateTestNamespace(t, sharedClusterName, namespaceCleanupTimeout)
	bundleDir := testutil.PreparePodinfoBundle(t, testEnv.podinfoPackagePath, "podinfo", namespace)
	markBundleRemoved := testutil.RegisterBundleCleanup(t, bundleDir, namespaceCleanupTimeout)
	artifactPath := testutil.CreateBundleArtifact(t, bundleDir)

	deployDir := t.TempDir()
	deployArtifact := filepath.Join(deployDir, filepath.Base(artifactPath))
	require.NoError(t, os.Rename(artifactPath, deployArtifact))

	testutil.RequireCLI(t,
		"bundle", "deploy", "--skip-signature-verification", deployArtifact,
		"--config", testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl"),
	)

	assertPodinfoConfiguration(t, k8s, namespace)
	stateSecret := testutil.ZarfPackageStateSecretName("podinfo", namespace)
	k8s.AssertSecretExists("zarf", stateSecret)
	result := testutil.RemoveBundle(t, deployArtifact, "--skip-signature-verification")
	markBundleRemoved()
	assert.Equal(t, "podinfo-cluster-test", result.BundleName)
	assert.Equal(t, []bundle.RemovePackageResult{{
		Name:   "podinfo",
		Status: bundle.RemovePackageStatusRemoved,
	}}, result.Packages)
	k8s.AssertDeploymentNotExists(namespace, "podinfo")
	k8s.AssertSecretNotExists("zarf", stateSecret)
}

func TestDeploySignedArtifact(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateTestNamespace(t, sharedClusterName, namespaceCleanupTimeout)
	bundleDir := testutil.PreparePodinfoBundle(t, testEnv.podinfoPackagePath, "podinfo-signed", namespace)
	testutil.RegisterBundleCleanup(t, bundleDir, namespaceCleanupTimeout)
	artifactPath := testutil.CreateBundleArtifact(t, bundleDir)
	privateKey, publicKey := testutil.GenerateCosignKeyPair(t)

	testutil.RequireCLI(t,
		"bundle", "sign", artifactPath, "--signing-key", privateKey,
	)
	testutil.RequireCLI(t,
		"bundle", "deploy", artifactPath,
		"--public-key", publicKey,
		"--config", testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl"),
	)

	assertPodinfoConfiguration(t, k8s, namespace)
}

func TestDeployFromOCIAndRemove(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateTestNamespace(t, sharedClusterName, namespaceCleanupTimeout)
	bundleDir := testutil.PreparePodinfoBundle(t, testEnv.podinfoPackagePath, "podinfo_oci_remove", namespace)
	markBundleRemoved := testutil.RegisterBundleCleanup(t, bundleDir, namespaceCleanupTimeout)
	artifactPath := testutil.CreateBundleArtifact(t, bundleDir)

	registryHost := testutil.StartLocalRegistry(t)
	ref := registryHost + "/test/podinfo-remove:v0.1.0"
	configPath := testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl")

	testutil.RequireCLI(t, "bundle", "push", artifactPath, ref, "--plain-http")
	testutil.RequireCLI(t,
		"bundle", "deploy", "--skip-signature-verification", "oci://"+ref,
		"--plain-http", "--config", configPath,
	)

	assertPodinfoConfiguration(t, k8s, namespace)
	stateSecret := testutil.ZarfPackageStateSecretName("podinfo", namespace)
	k8s.AssertSecretExists("zarf", stateSecret)
	result := testutil.RemoveBundle(t, "oci://"+ref,
		"--plain-http", "--skip-signature-verification",
	)
	markBundleRemoved()
	assert.Equal(t, "podinfo-cluster-test", result.BundleName)
	assert.Equal(t, []bundle.RemovePackageResult{{
		Name:   "podinfo_oci_remove",
		Status: bundle.RemovePackageStatusRemoved,
	}}, result.Packages)
	k8s.AssertDeploymentNotExists(namespace, "podinfo")
	k8s.AssertSecretNotExists("zarf", stateSecret)
}

func assertPodinfoConfiguration(t *testing.T, k8s *testutil.K8sClient, namespace string) {
	t.Helper()

	k8s.WaitForDeploymentReady(namespace, "podinfo", podinfoReadyTimeout)
	k8s.AssertDeploymentReplicas(namespace, "podinfo", 1)
	k8s.AssertServiceNotExists(namespace, "podinfo")
	k8s.AssertDeploymentPodAnnotation(namespace, "podinfo", "app.kubernetes.io/managed-by", "uds")
	k8s.AssertDeploymentPodAnnotation(namespace, "podinfo", "team", "platform")
	k8s.AssertDeploymentPodToleration(
		namespace,
		"podinfo",
		"node.kubernetes.io/not-ready",
		corev1.TolerationOpExists,
		corev1.TaintEffectNoExecute,
	)
}
