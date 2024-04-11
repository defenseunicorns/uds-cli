// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDevDeploy(t *testing.T) {

	removeZarfInit()
	cmd := strings.Split("zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")

	t.Run("Test dev deploy with local and remote pkgs", func(t *testing.T) {

		e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

		bundleDir := "src/test/bundles/03-local-and-remote"
		bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

		devDeploy(t, bundleDir)

		deployments, _, _ := e2e.UDS(cmd...)
		require.Contains(t, deployments, "podinfo")
		require.Contains(t, deployments, "nginx")

		remove(t, bundlePath)
	})

	t.Run("Test dev deploy with CreateLocalPkgs", func(t *testing.T) {

		e2e.DeleteZarfPkg(t, "src/test/packages/podinfo")

		bundleDir := "src/test/bundles/03-local-and-remote"
		bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

		devDeployPackages(t, bundleDir, "podinfo")

		deployments, _, _ := e2e.UDS(cmd...)
		require.Contains(t, deployments, "podinfo")
		require.NotContains(t, deployments, "nginx")

		remove(t, bundlePath)
	})
}
