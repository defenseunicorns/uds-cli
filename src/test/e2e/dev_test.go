// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDevDeploy(t *testing.T) {

	removeZarfInit()

	t.Run("Test dev deploy with local and remote pkgs", func(t *testing.T) {
		e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

		bundleDir := "src/test/bundles/03-local-and-remote"
		bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

		runCmd(t, fmt.Sprintf("dev deploy %s", bundleDir))

		deployments, _ := runCmd(t, "zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'")
		require.Contains(t, deployments, "podinfo")
		require.Contains(t, deployments, "nginx")

		runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundlePath))
	})

	t.Run("Test dev deploy with CreateLocalPkgs", func(t *testing.T) {
		e2e.DeleteZarfPkg(t, "src/test/packages/podinfo")

		bundleDir := "src/test/bundles/03-local-and-remote"
		bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

		runCmd(t, fmt.Sprintf("dev deploy %s --packages %s", bundleDir, "podinfo"))

		deployments, _ := runCmd(t, "zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'")
		require.Contains(t, deployments, "podinfo")
		require.NotContains(t, deployments, "nginx")

		runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundlePath))
	})

	t.Run("Test dev deploy with ref flag", func(t *testing.T) {
		e2e.DeleteZarfPkg(t, "src/test/packages/podinfo")
		bundleDir := "src/test/bundles/03-local-and-remote"

		runCmd(t, fmt.Sprintf("dev deploy %s --ref %s", bundleDir, "nginx=0.0.2"))

		ref, _ := runCmd(t, "zarf tools kubectl get deployment -n nginx nginx-deployment -o=jsonpath='{.spec.template.spec.containers[0].image}'")
		require.Contains(t, ref, "nginx:1.26.0")

		runCmd(t, "zarf tools kubectl delete ns podinfo nginx zarf")
	})

	t.Run("Test dev deploy with flavor flag", func(t *testing.T) {
		e2e.DeleteZarfPkg(t, "src/test/packages/podinfo/flavors")
		bundleDir := "src/test/bundles/15-dev-deploy"

		runCmd(t, fmt.Sprintf("dev deploy %s --flavor %s", bundleDir, "podinfo=patchVersion3"))

		ref, _ := runCmd(t, "zarf tools kubectl get deployment -n podinfo-flavor podinfo -o=jsonpath='{.spec.template.spec.containers[0].image}'")
		require.Contains(t, ref, "ghcr.io/stefanprodan/podinfo:6.6.3")

		runCmd(t, "zarf tools kubectl delete ns zarf podinfo-flavor")
	})

	t.Run("Test dev deploy with global flavor", func(t *testing.T) {
		bundleDir := "src/test/bundles/15-dev-deploy"

		runCmd(t, fmt.Sprintf("dev deploy %s --flavor %s --force-create", bundleDir, "patchVersion3"))

		ref, _ := runCmd(t, "zarf tools kubectl get deployment -n podinfo-flavor podinfo -o=jsonpath='{.spec.template.spec.containers[0].image}'")
		require.Contains(t, ref, "ghcr.io/stefanprodan/podinfo:6.6.3")

		runCmd(t, "zarf tools kubectl delete ns zarf podinfo-flavor")
	})

	t.Run("Test dev deploy with flavor and force create", func(t *testing.T) {

		bundleDir := "src/test/bundles/15-dev-deploy"

		// create flavor patchVersion3 podinfo-flavor package
		pkgDir := "src/test/packages/podinfo"

		runCmd(t, fmt.Sprintf("zarf package create %s --flavor %s --confirm -o %s", pkgDir, "patchVersion3", pkgDir))

		// dev deploy with flavor patchVersion2 and --force-create
		runCmd(t, fmt.Sprintf("dev deploy %s --flavor %s --force-create", bundleDir, "podinfo=patchVersion2"))

		ref, _ := runCmd(t, "zarf tools kubectl get deployment -n podinfo-flavor podinfo -o=jsonpath='{.spec.template.spec.containers[0].image}'")
		// assert that podinfo package with flavor patchVersion2 was deployed.
		require.Contains(t, ref, "ghcr.io/stefanprodan/podinfo:6.6.2")

		runCmd(t, "zarf tools kubectl delete ns zarf podinfo-flavor")
	})
	t.Run("Test dev deploy with remote bundle", func(t *testing.T) {
		bundle := "oci://ghcr.io/defenseunicorns/packages/uds-cli/test/publish/ghcr-test:0.0.1"
		runCmd(t, fmt.Sprintf("dev deploy %s", bundle))

		deployments, _ := runCmd(t, "zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'")
		require.Contains(t, deployments, "podinfo")
		require.Contains(t, deployments, "nginx")

		runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundle))
	})

	t.Run("Test dev deploy with --set flag", func(t *testing.T) {
		bundleDir := "src/test/bundles/02-variables"
		bundleTarballPath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-variables-%s-0.0.1.tar.zst", e2e.Arch))
		_, stderr := runCmd(t, "dev deploy "+bundleDir+" --set ANIMAL=Longhorns --set COUNTRY=Texas -l=debug")
		require.Contains(t, stderr, "This fun-fact was imported: Longhorns are the national animal of Texas")
		require.NotContains(t, stderr, "This fun-fact was imported: Unicorns are the national animal of Scotland")
		runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundleTarballPath))
	})

	// delete packages because other tests depend on them being created with SBOMs (ie. force other tests to re-create)
	e2e.DeleteZarfPkg(t, "src/test/packages/podinfo")
	e2e.DeleteZarfPkg(t, "src/test/packages/nginx")
}
