// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster_test

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/stretchr/testify/assert"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

// TestDeployAndRemoveBundle preserves the original full deploy-remove coverage
// without removing the suite's shared k3d and Zarf bootstrap. Both HCL package
// labels intentionally differ from the underlying Zarf metadata.name, while
// namespace overrides give each deployment independent cluster state.
func TestDeployAndRemoveBundle(t *testing.T) {
	t.Parallel()

	firstNamespace, firstK8s := testutil.AllocateTestNamespace(t, sharedClusterName, namespaceCleanupTimeout)
	secondNamespace, secondK8s := testutil.AllocateTestNamespace(t, sharedClusterName, namespaceCleanupTimeout)
	bundleDir := testutil.PrepareTwoPodinfoBundle(t, testEnv.podinfoPackagePath, firstNamespace, secondNamespace)
	markBundleRemoved := testutil.RegisterBundleCleanup(t, testEnv.udsPath, bundleDir, namespaceCleanupTimeout)

	testutil.RequireUDSCommand(t, testEnv.udsPath,
		"bundle", "dev", "deploy", bundleDir,
		"--config", testutil.TestDataPath("bundles/deploy/variables/full-config.uds.hcl"),
	)
	firstK8s.WaitForDeploymentReady(firstNamespace, "podinfo", podinfoReadyTimeout)
	secondK8s.WaitForDeploymentReady(secondNamespace, "podinfo", podinfoReadyTimeout)
	firstStateSecret := testutil.ZarfPackageStateSecretName("podinfo", firstNamespace)
	secondStateSecret := testutil.ZarfPackageStateSecretName("podinfo", secondNamespace)
	firstK8s.AssertSecretExists("zarf", firstStateSecret)
	secondK8s.AssertSecretExists("zarf", secondStateSecret)

	result := testutil.RemoveBundle(t, testEnv.udsPath, bundleDir)
	markBundleRemoved()
	assert.Equal(t, "k3d-core-init", result.BundleName)
	assert.ElementsMatch(t, []bundle.RemovePackageResult{
		{Name: "pod_info_primary", Status: bundle.RemovePackageStatusRemoved},
		{Name: "pod_info_secondary", Status: bundle.RemovePackageStatusRemoved},
	}, result.Packages)
	firstK8s.AssertDeploymentNotExists(firstNamespace, "podinfo")
	secondK8s.AssertDeploymentNotExists(secondNamespace, "podinfo")
	firstK8s.AssertSecretNotExists("zarf", firstStateSecret)
	secondK8s.AssertSecretNotExists("zarf", secondStateSecret)

	// Removing a test package must not disturb the suite-level Zarf installation.
	firstK8s.AssertSecretExists("zarf", zarfStateSecret)
	firstK8s.AssertDeploymentExists("zarf", zarfAgentDeployment)
}

func TestDeployAndRemoveBundleFromTarball(t *testing.T) {
	t.Parallel()
	namespace, k8s := testutil.AllocateTestNamespace(t, sharedClusterName, namespaceCleanupTimeout)
	bundleDir := testutil.PreparePodinfoBundle(t, testEnv.podinfoPackagePath, "podinfo_tarball_remove", namespace)
	markBundleRemoved := testutil.RegisterBundleCleanup(t, testEnv.udsPath, bundleDir, namespaceCleanupTimeout)
	artifactPath := testutil.CreateBundleArtifact(t, testEnv.udsPath, bundleDir)
	configPath := testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl")
	testutil.RequireUDSCommand(t, testEnv.udsPath,
		"bundle", "deploy", artifactPath,
		"--skip-signature-verification", "--config", configPath,
	)
	k8s.WaitForDeploymentReady(namespace, "podinfo", podinfoReadyTimeout)
	stateSecret := testutil.ZarfPackageStateSecretName("podinfo", namespace)
	k8s.AssertSecretExists("zarf", stateSecret)
	result := testutil.RemoveBundle(t, testEnv.udsPath, artifactPath, "--skip-signature-verification")
	markBundleRemoved()
	assert.Equal(t, "podinfo-cluster-test", result.BundleName)
	assert.Equal(t, []bundle.RemovePackageResult{{
		Name:   "podinfo_tarball_remove",
		Status: bundle.RemovePackageStatusRemoved,
	}}, result.Packages)
	k8s.AssertDeploymentNotExists(namespace, "podinfo")
	k8s.AssertSecretNotExists("zarf", stateSecret)
}
func TestDeployAndRemoveBundleFromOCI(t *testing.T) {
	t.Parallel()
	namespace, k8s := testutil.AllocateTestNamespace(t, sharedClusterName, namespaceCleanupTimeout)
	bundleDir := testutil.PreparePodinfoBundle(t, testEnv.podinfoPackagePath, "podinfo_oci_remove", namespace)
	markBundleRemoved := testutil.RegisterBundleCleanup(t, testEnv.udsPath, bundleDir, namespaceCleanupTimeout)
	artifactPath := testutil.CreateBundleArtifact(t, testEnv.udsPath, bundleDir)
	registryHost := testutil.StartLocalRegistry(t)
	ref := registryHost + "/test/podinfo-remove:v0.1.0"
	configPath := testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl")
	testutil.RequireUDSCommand(t, testEnv.udsPath,
		"bundle", "push", artifactPath, ref, "--plain-http",
	)
	testutil.RequireUDSCommand(t, testEnv.udsPath,
		"bundle", "deploy", "oci://"+ref,
		"--plain-http", "--skip-signature-verification", "--config", configPath,
	)
	k8s.WaitForDeploymentReady(namespace, "podinfo", podinfoReadyTimeout)
	stateSecret := testutil.ZarfPackageStateSecretName("podinfo", namespace)
	k8s.AssertSecretExists("zarf", stateSecret)
	result := testutil.RemoveBundle(t, testEnv.udsPath, "oci://"+ref,
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
