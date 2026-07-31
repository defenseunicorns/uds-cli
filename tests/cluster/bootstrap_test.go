// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster_test

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

// TestZarfInitialized verifies the suite bootstrap produced a functional
// cluster with the expected Zarf resources.
func TestZarfInitialized(t *testing.T) {
	t.Parallel()

	k8s := testutil.NewK8sClientOrFail(t)
	k8s.AssertNamespaceExists("zarf")
	k8s.AssertSecretExists("zarf", zarfStateSecret)
	k8s.AssertDeploymentExists("zarf", zarfRegistryDeployment)
	k8s.AssertDeploymentExists("zarf", zarfAgentDeployment)
}
