// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMonitor(t *testing.T) {
	// this test assumes a running k3d cluster with the UDS operator and admission controller installed
	// recommend running with uds run test:engine-e2e to install controllers
	cmd := strings.Split("zarf tools kubectl get deployments -n pepr-system -o=jsonpath='{.items[*].metadata.name}'", " ")
	deployments, _, _ := e2e.UDS(cmd...)
	require.Contains(t, deployments, "pepr-uds-core")
	require.Contains(t, deployments, "pepr-uds-core-watcher")

	// we expect this command to fail because UDS Core doesn't allow some of the configs in this package
	_, _, err := e2e.UDS("zarf", "dev", "deploy", "src/test/packages/engine", "--retries=1")
	require.Error(t, err)

	t.Run("test mutated policies", func(t *testing.T) {
		stdout, _, _ := e2e.UDS("monitor", "pepr", "mutated")
		require.Contains(t, stdout, "✎ MUTATED   podinfo")
	})

	t.Run("test allowed policies", func(t *testing.T) {
		stdout, _, _ := e2e.UDS("monitor", "pepr", "allowed")
		require.Contains(t, stdout, "✓ ALLOWED   podinfo")
	})

	t.Run("test denied policies", func(t *testing.T) {
		stdout, _, _ := e2e.UDS("monitor", "pepr", "denied")
		require.Contains(t, stdout, "✗ DENIED    podinfo")
	})
}
