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

func TestUDSCoreStandardSmoke(t *testing.T) {
	cluster := testutil.CreateDisposableCluster(t, "uds-core")
	result := testutil.RunCLI(t, cluster.CommandOptions(),
		"deploy", cluster.BundlePath, "--confirm", "--no-progress")
	require.NoError(t, result.Err, result.Stderr)

	kubernetes := cluster.Kubernetes()
	for _, deployment := range []struct {
		namespace string
		name      string
	}{
		{namespace: "pepr-system", name: "pepr-uds-core"},
		{namespace: "pepr-system", name: "pepr-uds-core-watcher"},
		{namespace: "istio-admin-gateway", name: "admin-ingressgateway"},
		{namespace: "istio-tenant-gateway", name: "tenant-ingressgateway"},
	} {
		deployment := deployment
		t.Run("deployment/"+deployment.name, func(t *testing.T) {
			kubernetes.WaitForDeploymentReady(deployment.namespace, deployment.name, 15*time.Minute)
			kubernetes.WaitForDeploymentPodsReady(deployment.namespace, deployment.name, 5*time.Minute)
		})
	}

	t.Run("statefulset/keycloak", func(t *testing.T) {
		kubernetes.WaitForStatefulSetReady("keycloak", "keycloak", 15*time.Minute)
		kubernetes.WaitForStatefulSetPodsReady("keycloak", "keycloak", 5*time.Minute)
	})

	t.Run("resource-requests/pepr", func(t *testing.T) {
		kubernetes.AssertDeploymentContainerRequests("pepr-system", "pepr-uds-core")
		kubernetes.AssertDeploymentContainerRequests("pepr-system", "pepr-uds-core-watcher")
	})
	t.Run("resource-requests/keycloak", func(t *testing.T) {
		kubernetes.AssertStatefulSetContainerRequests("keycloak", "keycloak")
	})

	for _, resource := range []struct {
		resource  string
		name      string
		namespace string
	}{
		{resource: "gateways.networking.istio.io", name: "admin-gateway", namespace: "istio-admin-gateway"},
		{resource: "gateways.networking.istio.io", name: "tenant-gateway", namespace: "istio-tenant-gateway"},
		{resource: "package", name: "keycloak", namespace: "keycloak"},
	} {
		resource := resource
		t.Run("resource/"+resource.name, func(t *testing.T) {
			result := testutil.RunCLI(t, cluster.CommandOptions(), "zarf", "tools", "wait-for",
				resource.resource, resource.name, "-n", resource.namespace, "--timeout", "30s")
			require.NoError(t, result.Err, result.Stderr)
		})
	}
}
