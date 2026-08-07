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

// TestMonitorUDSCore is smoke coverage because monitor events require the UDS
// Core Pepr controllers; a generic Zarf-initialized cluster cannot produce them.
func TestMonitorUDSCore(t *testing.T) {
	cluster := testutil.CreateDisposableCluster(t, "monitor-uds-core")
	deployed := testutil.RunCLI(t, cluster.CommandOptions(),
		"deploy", cluster.BundlePath, "--confirm", "--no-progress")
	require.NoError(t, deployed.Err, deployed.Stderr)

	k8s := cluster.Kubernetes()
	k8s.WaitForDeploymentReady("pepr-system", "pepr-uds-core", 15*time.Minute)
	k8s.WaitForDeploymentReady("pepr-system", "pepr-uds-core-watcher", 15*time.Minute)

	engine := testutil.RunCLI(t, cluster.CommandOptions(),
		"zarf", "dev", "deploy", testutil.TestDataPath("packages/engine"), "--retries=1")
	require.Error(t, engine.Err)

	for _, event := range []struct {
		name   string
		marker string
	}{
		{name: "mutated", marker: "✎ MUTATED   podinfo"},
		{name: "allowed", marker: "✓ ALLOWED   podinfo"},
		{name: "denied", marker: "✗ DENIED    podinfo"},
	} {
		event := event
		t.Run(event.name, func(t *testing.T) {
			result := testutil.RunCLI(t, cluster.CommandOptions(), "monitor", "pepr", event.name)
			require.NoError(t, result.Err, result.Stderr)
			require.Contains(t, result.Stdout, event.marker)
		})
	}
}
