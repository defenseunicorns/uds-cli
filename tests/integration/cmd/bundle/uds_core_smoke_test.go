// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build uds_core_smoke

package bundle_test

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

const udsCoreReadyTimeout = 15 * time.Minute
const udsCoreClusterName = "uds"

type UDSCoreSmokeSuite struct {
	suite.Suite
	uds string
}

func TestUDSCoreSmokeSuite(t *testing.T) {
	suite.Run(t, new(UDSCoreSmokeSuite))
}

func (s *UDSCoreSmokeSuite) SetupSuite() {
	s.uds = testutil.UDSCLIPath(s.T(), "run via 'maru run test:uds-core-smoke'")
}

func (s *UDSCoreSmokeSuite) TearDownTest() {
	testutil.DeleteK3dCluster(s.T(), udsCoreClusterName)
}

func (s *UDSCoreSmokeSuite) TestUDSCore_StandardSmoke_CreateAndDeploy() {
	testutil.CheckDockerRunning(s.T(), "Docker is not running; UDS Core smoke tests require Docker for k3d")

	deployArtifact := testutil.CreateBundleFromTestDataCLIWithBinary(s.T(), s.uds, "bundles/uds-core/standard", runtime.GOARCH)
	testutil.RunBundleDeploy(s.T(), s.uds, deployArtifact)

	k8s := testutil.NewK8sClientOrFail(s.T())

	k8s.AssertNamespaceExists("zarf")
	k8s.AssertSecretExists("zarf", "zarf-state")

	k8s.WaitForDeploymentReady("pepr-system", "pepr-uds-core", udsCoreReadyTimeout)
	k8s.WaitForDeploymentReady("pepr-system", "pepr-uds-core-watcher", udsCoreReadyTimeout)
	k8s.WaitForDeploymentReady("istio-system", "istiod", udsCoreReadyTimeout)
	k8s.WaitForDeploymentReady("istio-admin-gateway", "admin-ingressgateway", udsCoreReadyTimeout)
	k8s.WaitForDeploymentReady("istio-tenant-gateway", "tenant-ingressgateway", udsCoreReadyTimeout)
	k8s.WaitForDeploymentReady("istio-passthrough-gateway", "passthrough-ingressgateway", udsCoreReadyTimeout)
	k8s.WaitForDeploymentReady("istio-egress-gateway", "egressgateway", udsCoreReadyTimeout)
	k8s.WaitForDeploymentReady("envoy-gateway-system", "envoy-gateway", udsCoreReadyTimeout)
	k8s.WaitForDeploymentReady("metrics-server", "metrics-server", udsCoreReadyTimeout)
	k8s.WaitForStatefulSetReady("keycloak", "keycloak", udsCoreReadyTimeout)
	k8s.WaitForDeploymentReady("authservice", "authservice", udsCoreReadyTimeout)
	k8s.WaitForStatefulSetReady("loki", "loki-write", udsCoreReadyTimeout)
	k8s.WaitForStatefulSetReady("loki", "loki-backend", udsCoreReadyTimeout)
	k8s.WaitForDeploymentReady("loki", "loki-read", udsCoreReadyTimeout)
	k8s.WaitForDeploymentReady("grafana", "grafana", udsCoreReadyTimeout)
	k8s.WaitForDeploymentReady("falco", "falco-falcosidekick", udsCoreReadyTimeout)
	k8s.WaitForReadyPodBySelector("monitoring", "app.kubernetes.io/name=prometheus", udsCoreReadyTimeout)
	k8s.WaitForReadyPodBySelector("monitoring", "app.kubernetes.io/name=alertmanager", udsCoreReadyTimeout)

	k8s.AssertDeploymentContainerRequests("pepr-system", "pepr-uds-core", "server", "200m", "256Mi")
	k8s.AssertDeploymentContainerRequests("pepr-system", "pepr-uds-core-watcher", "watcher", "200m", "256Mi")
	k8s.AssertDeploymentReplicas("loki", "loki-read", 1)
	k8s.AssertStatefulSetReplicas("loki", "loki-write", 1)
	k8s.AssertStatefulSetReplicas("loki", "loki-backend", 1)
}
