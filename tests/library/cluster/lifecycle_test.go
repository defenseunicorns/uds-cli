// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library && cluster_integration && owned_cluster

package cluster_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

// TestClusterLifecycle owns a complete cluster lifecycle and bootstraps Zarf
// through the public bundle API. It is intentionally serial because it mutates
// KUBECONFIG and Docker-managed cluster state.
func TestClusterLifecycle(t *testing.T) {
	before := clusterNames(t)
	name, kubeconfig := createCluster(t)
	t.Setenv("KUBECONFIG", kubeconfig)

	streams := iostreams.IOStreams{}
	sourcePath := initBundlePath(t)
	source, err := bundle.PrepareDeploySource(t.Context(), streams, sourcePath, t.TempDir(), runtime.GOARCH)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Errorf("close prepared init source: %v", err)
		}
	})

	config := &bundle.UDSBundleConfig{Options: &bundle.ConfigOptions{
		Architecture: runtime.GOARCH,
		Concurrency:  1,
		LogLevel:     "info",
		TmpDir:       t.TempDir(),
	}}
	ctx, cancel := context.WithTimeout(t.Context(), clusterCreateTimeout)
	defer cancel()
	result, err := bundle.Deploy(ctx, source, bundle.DeployOptions{Config: config, Streams: streams})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "init-no-k3s", result.BundleName)
	require.Len(t, result.Packages, 1)
	require.Equal(t, "init", result.Packages[0].Name)

	client := kubernetesClient(t, kubeconfig)
	requireZarfReady(t, client)

	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), clusterCleanupTimeout)
	defer cleanupCancel()
	require.NoError(t, deleteCluster(cleanupCtx, name))
	require.NotContains(t, clusterNames(t), name)

	after := clusterNames(t)
	for existing := range before {
		require.Contains(t, after, existing, "pre-existing cluster must survive owned-cluster cleanup")
	}
}
