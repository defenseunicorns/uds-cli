// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster

import (
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/stretchr/testify/require"
)

// TestRemoteBundleDeploysAndRemoves verifies the OCI create, publish, pull-on-deploy,
// and removal path using a test-owned registry and namespace.
func TestRemoteBundleDeploysAndRemoves(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	registry := testutil.NewRegistry(t)
	bundleDir := prepareLocalAndRemoteBundle(t, opts, namespace)
	bundlePath := createClusterBundle(t, opts, bundleDir, "../../packages/podinfo")
	remoteRepository := registry.Host + "/remote-bundle"
	remoteRef := "oci://" + remoteRepository + "/test-local-and-remote:0.0.1"
	published := runClusterCLI(t, opts, "publish", bundlePath, remoteRepository, "--insecure")
	require.NoError(t, published.Err, published.Stderr)
	defer removeBundle(t, opts, remoteRef, nil)

	deployed := runClusterCLI(t, opts, "deploy", remoteRef, "--confirm", "--insecure")
	require.NoError(t, deployed.Err, deployed.Stderr)
	k8s.WaitForDeploymentReady(namespace, "podinfo", deployFlagsTimeout)
	k8s.WaitForDeploymentReady(namespace, "nginx-deployment", deployFlagsTimeout)

	pullDir := filepath.Join(t.TempDir(), "pulled")
	pulled := runClusterCLI(t, opts, "pull", remoteRef, "--insecure", "--output", pullDir)
	require.NoError(t, pulled.Err, pulled.Stderr)
}
