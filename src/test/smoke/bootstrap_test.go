// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build uds_core_smoke

package smoke

import (
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestFirstInstallBootstrap(t *testing.T) {
	cluster := testutil.CreateDisposableCluster(t, "uds-bootstrap")
	result := testutil.RunCLI(t, cluster.CommandOptions(),
		"deploy", cluster.BundlePath, "--packages", "init", "--confirm", "--no-progress")
	require.NoError(t, result.Err, result.Stderr)

	kubernetes := cluster.Kubernetes()
	kubernetes.AssertNamespaceExists("zarf")
	for _, deployment := range []string{"zarf-docker-registry", "agent-hook"} {
		kubernetes.WaitForDeploymentReady("zarf", deployment, 10*time.Minute)
		kubernetes.WaitForDeploymentPodsReady("zarf", deployment, 2*time.Minute)
	}
}
