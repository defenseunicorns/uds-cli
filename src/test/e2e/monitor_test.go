// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test provides e2e tests for UDS.
package test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMonitor(t *testing.T) {
	// this test assumes a running cluster with the UDS installed (i.e. k3d-core-slim-dev)
	// recommend running this test after deploying with `uds deploy k3d-core-slim-dev:latest --packages uds-k3d-dev,init,core-base`
	deployments, _ := runCmd(t, "zarf tools kubectl get deployments -n pepr-system -o=jsonpath='{.items[*].metadata.name}'")
	require.Contains(t, deployments, "pepr-uds-core")
	require.Contains(t, deployments, "pepr-uds-core-watcher")

	// we expect this command to fail because UDS Core doesn't allow some of the configs in this package
	_, _, err := runCmdWithErr("zarf dev deploy src/test/packages/engine --retries=1")
	require.Error(t, err)

	t.Run("test mutated policies", func(t *testing.T) {
		stdout, _ := runCmd(t, "monitor pepr mutated")
		require.Contains(t, stdout, "✎ MUTATED   podinfo")
	})

	t.Run("test allowed policies", func(t *testing.T) {
		stdout, _ := runCmd(t, "monitor pepr allowed")
		require.Contains(t, stdout, "✓ ALLOWED   podinfo")
	})

	t.Run("test denied policies", func(t *testing.T) {
		stdout, _ := runCmd(t, "monitor pepr denied")
		require.Contains(t, stdout, "✗ DENIED    podinfo")
	})
}
