// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package lifecycle_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
	"github.com/stretchr/testify/require"
)

const (
	lifecycleTimeout = 20 * time.Minute
	cleanupTimeout   = 5 * time.Minute
)

func TestCLILifecycle(t *testing.T) {
	testutil.RequireZarfVersion(t)
	testutil.CheckDockerRunning(t, "Docker is not running; cluster lifecycle tests require Docker for k3d")

	before := clusterNames(t, t.Context())
	name := "uds-cli-owned-" + randomSuffix(t)
	require.NotContains(t, before, name)
	workDir := t.TempDir()
	kubeconfig := filepath.Join(workDir, "kubeconfig.yaml")
	t.Setenv("KUBECONFIG", kubeconfig)

	apiPort, err := testutil.AvailableTCPPort()
	require.NoError(t, err)
	configPath := filepath.Join(workDir, "config.uds.hcl")
	require.NoError(t, testutil.WriteBootstrapConfig(configPath, name, apiPort))

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		require.NoError(t, testutil.DeleteK3dClusterContext(ctx, name), "delete owned cluster %q", name)
		after := clusterNames(t, ctx)
		require.NotContains(t, after, name, "owned cluster must be deleted")
		for existing := range before {
			require.Contains(t, after, existing, "pre-existing cluster must survive owned-cluster cleanup")
		}
	})

	ctx, cancel := context.WithTimeout(t.Context(), lifecycleTimeout)
	defer cancel()
	bootstrap := testutil.TestDataPath("bundles/deploy/init")
	runCLI(t, ctx,
		"bundle", "dev", "deploy", bootstrap,
		"--config", configPath,
	)

	require.Contains(t, clusterNames(t, t.Context()), name, "CLI bootstrap must create the owned cluster")
	k8s := testutil.NewK8sClientOrFail(t)
	k8s.AssertNamespaceExists("zarf")
	k8s.AssertSecretExists("zarf", "zarf-state")
	k8s.AssertDeploymentExists("zarf", "zarf-docker-registry")
	k8s.AssertDeploymentExists("zarf", "agent-hook")
}

func runCLI(t *testing.T, ctx context.Context, args ...string) {
	t.Helper()
	var stdout, stderr strings.Builder
	err := testutil.ExecuteCLI(ctx, iostreams.New(nil, &stdout, &stderr), args...)
	require.NoError(t, err, "UDS CLI command %q failed:\n%s", strings.Join(args, " "), stdout.String()+stderr.String())
}

func clusterNames(t *testing.T, ctx context.Context) map[string]struct{} {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "k3d", "cluster", "list", "--no-headers")
	output, err := command.Output()
	require.NoError(t, err, "list k3d clusters")
	names := map[string]struct{}{}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			names[fields[0]] = struct{}{}
		}
	}
	return names
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	value := make([]byte, 8)
	_, err := rand.Read(value)
	require.NoError(t, err)
	return hex.EncodeToString(value)
}
