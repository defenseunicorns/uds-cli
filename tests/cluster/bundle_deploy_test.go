// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

const podinfoReadyTimeout = 5 * time.Minute

func TestDeployVariablesBundleWithPodinfo(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateTestNamespace(t, sharedClusterName, namespaceCleanupTimeout)
	bundleDir := testutil.PreparePodinfoBundle(t, testEnv.podinfoPackagePath, "podinfo", namespace)
	testutil.RegisterBundleCleanup(t, testEnv.udsPath, bundleDir, namespaceCleanupTimeout)

	testutil.RequireUDSCommand(t, testEnv.udsPath,
		"bundle", "dev", "deploy", bundleDir,
		"--config", testutil.TestDataPath("bundles/deploy/variables/full-config.uds.hcl"),
	)

	assertPodinfoConfiguration(t, k8s, namespace)
}

func TestDeployFromArtifact(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateTestNamespace(t, sharedClusterName, namespaceCleanupTimeout)
	bundleDir := testutil.PreparePodinfoBundle(t, testEnv.podinfoPackagePath, "podinfo", namespace)
	testutil.RegisterBundleCleanup(t, testEnv.udsPath, bundleDir, namespaceCleanupTimeout)
	artifactPath := testutil.CreateBundleArtifact(t, testEnv.udsPath, bundleDir)

	deployDir := t.TempDir()
	deployArtifact := filepath.Join(deployDir, filepath.Base(artifactPath))
	require.NoError(t, os.Rename(artifactPath, deployArtifact))

	testutil.RequireUDSCommand(t, testEnv.udsPath,
		"bundle", "deploy", deployArtifact,
		"--config", testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl"),
	)

	assertPodinfoConfiguration(t, k8s, namespace)
}

func TestDeployFromOCI(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateTestNamespace(t, sharedClusterName, namespaceCleanupTimeout)
	bundleDir := testutil.PreparePodinfoBundle(t, testEnv.podinfoPackagePath, "podinfo_oci", namespace)
	testutil.RegisterBundleCleanup(t, testEnv.udsPath, bundleDir, namespaceCleanupTimeout)
	artifactPath := testutil.CreateBundleArtifact(t, testEnv.udsPath, bundleDir)

	registryServer := httptest.NewServer(registry.New())
	t.Cleanup(registryServer.Close)
	registryHost := strings.TrimPrefix(registryServer.URL, "http://")
	ref := registryHost + "/test/podinfo-oci:v0.1.0"
	configPath := testutil.TestDataPath("bundles/deploy/variables/config.uds.hcl")

	testutil.RequireUDSCommand(t, testEnv.udsPath, "bundle", "push", artifactPath, ref, "--plain-http")
	testutil.RequireUDSCommand(t, testEnv.udsPath,
		"bundle", "deploy", "oci://"+ref,
		"--plain-http", "--config", configPath,
	)

	assertPodinfoConfiguration(t, k8s, namespace)
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
