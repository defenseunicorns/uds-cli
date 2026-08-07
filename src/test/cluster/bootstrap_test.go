// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestZarfInitialized(t *testing.T) {
	t.Parallel()

	client := testutil.NewKubernetes(t, suite.KubeconfigPath)
	client.AssertNamespaceExists("zarf")
	client.AssertDeploymentExists("zarf", "zarf-docker-registry")
	client.AssertDeploymentExists("zarf", "agent-hook")
}

func TestAllocateNamespaceReservesAndCleansUpNamespace(t *testing.T) {
	t.Parallel()

	client := testutil.NewKubernetes(t, suite.KubeconfigPath)
	var namespaces []string
	t.Run("allocate", func(t *testing.T) {
		for range 2 {
			namespace, allocatedClient := testutil.AllocateNamespace(t, suite)
			require.Regexp(t, `^uds-it-[a-z0-9-]+-[0-9a-f]{8}$`, namespace)
			require.LessOrEqual(t, len(namespace), 63)
			allocatedClient.AssertNamespaceDoesNotExist(namespace)
			namespaces = append(namespaces, namespace)
		}
		require.NotEqual(t, namespaces[0], namespaces[1])
	})
	for _, namespace := range namespaces {
		client.AssertNamespaceDoesNotExist(namespace)
	}
}
