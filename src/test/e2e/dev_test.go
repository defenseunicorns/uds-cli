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

	t.Run("Test dev deploy with remote bundle", func(t *testing.T) {

		bundle := "oci://ghcr.io/defenseunicorns/packages/uds-cli/test/publish/ghcr-test:0.0.1"

		devDeploy(t, bundle)

		deployments, _, _ := e2e.UDS(cmd...)
		require.Contains(t, deployments, "podinfo")
		require.Contains(t, deployments, "nginx")

		remove(t, bundle)
	})

	t.Run("Test dev deploy with --set flag", func(t *testing.T) {
		bundleDir := "src/test/bundles/02-variables"
		bundleTarballPath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-variables-%s-0.0.1.tar.zst", e2e.Arch))
		_, stderr := runCmd(t, "dev deploy "+bundleDir+" --set ANIMAL=Longhorns --set COUNTRY=Texas --confirm -l=debug")
		require.Contains(t, stderr, "This fun-fact was imported: Longhorns are the national animal of Texas")
		require.NotContains(t, stderr, "This fun-fact was imported: Unicorns are the national animal of Scotland")
		remove(t, bundleTarballPath)
	})

	// delete packages because other tests depend on them being created with SBOMs (ie. force other tests to re-create)
	e2e.DeleteZarfPkg(t, "src/test/packages/podinfo")
	e2e.DeleteZarfPkg(t, "src/test/packages/nginx")
}
