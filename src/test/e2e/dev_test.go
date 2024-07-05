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

	// delete packages because other tests depend on them being created with SBOMs (ie. force other tests to re-create)
	defer e2e.DeleteZarfPkg(t, "src/test/packages/podinfo")
	defer e2e.DeleteZarfPkg(t, "src/test/packages/nginx")

	t.Run("Test dev deploy with local and remote pkgs", func(t *testing.T) {

		e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

		bundleDir := "src/test/bundles/03-local-and-remote"
		bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

		devDeploy(t, bundleDir)

		cmd := strings.Split("zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
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

		cmd := strings.Split("zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
		deployments, _, _ := e2e.UDS(cmd...)
		require.Contains(t, deployments, "podinfo")
		require.NotContains(t, deployments, "nginx")

		remove(t, bundlePath)
	})

	t.Run("Test dev deploy with ref flag", func(t *testing.T) {
		e2e.DeleteZarfPkg(t, "src/test/packages/podinfo")
		bundleDir := "src/test/bundles/03-local-and-remote"

		cmd := strings.Split(fmt.Sprintf("dev deploy %s --ref %s", bundleDir, "nginx=0.0.2"), " ")
		_, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl get deployment -n nginx nginx-deployment -o=jsonpath='{.spec.template.spec.containers[0].image}'", " ")
		ref, _, err := e2e.UDS(cmd...)
		require.Contains(t, ref, "nginx:1.26.0")
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl delete ns podinfo nginx zarf", " ")
		_, _, err = e2e.UDS(cmd...)
		require.NoError(t, err)
	})

	t.Run("Test dev deploy with flavor flag", func(t *testing.T) {
		e2e.DeleteZarfPkg(t, "src/test/packages/podinfo/flavors")
		bundleDir := "src/test/bundles/15-dev-deploy"

		cmd := strings.Split(fmt.Sprintf("dev deploy %s --flavor %s", bundleDir, "podinfo=patchVersion3"), " ")
		_, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl get deployment -n podinfo-flavor podinfo -o=jsonpath='{.spec.template.spec.containers[0].image}'", " ")
		ref, _, err := e2e.UDS(cmd...)
		require.Contains(t, ref, "ghcr.io/stefanprodan/podinfo:6.6.3")
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl delete ns zarf podinfo-flavor", " ")
		_, _, err = e2e.UDS(cmd...)
		require.NoError(t, err)
	})
	t.Run("Test dev deploy with global flavor", func(t *testing.T) {
		bundleDir := "src/test/bundles/15-dev-deploy"

		cmd := strings.Split(fmt.Sprintf("dev deploy %s --flavor %s --force-create", bundleDir, "patchVersion3"), " ")
		_, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl get deployment -n podinfo-flavor podinfo -o=jsonpath='{.spec.template.spec.containers[0].image}'", " ")
		ref, _, err := e2e.UDS(cmd...)
		require.Contains(t, ref, "ghcr.io/stefanprodan/podinfo:6.6.3")
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl delete ns zarf podinfo-flavor", " ")
		_, _, err = e2e.UDS(cmd...)
		require.NoError(t, err)
	})

	t.Run("Test dev deploy with flavor and force create", func(t *testing.T) {

		bundleDir := "src/test/bundles/15-dev-deploy"

		// create flavor patchVersion3 podinfo-flavor package
		pkgDir := "src/test/packages/podinfo"
		cmd := strings.Split(fmt.Sprintf("zarf package create %s --flavor %s --confirm -o %s", pkgDir, "patchVersion3", pkgDir), " ")
		_, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)

		// dev deploy with flavor patchVersion2 and --force-create
		cmd = strings.Split(fmt.Sprintf("dev deploy %s --flavor %s --force-create", bundleDir, "podinfo=patchVersion2"), " ")
		_, _, err = e2e.UDS(cmd...)
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl get deployment -n podinfo-flavor podinfo -o=jsonpath='{.spec.template.spec.containers[0].image}'", " ")
		ref, _, err := e2e.UDS(cmd...)
		// assert that podinfo package with flavor patchVersion2 was deployed.
		require.Contains(t, ref, "ghcr.io/stefanprodan/podinfo:6.6.2")
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl delete ns zarf podinfo-flavor", " ")
		_, _, err = e2e.UDS(cmd...)
		require.NoError(t, err)
	})
	t.Run("Test dev deploy with remote bundle", func(t *testing.T) {
		bundle := "oci://ghcr.io/defenseunicorns/packages/uds-cli/test/publish/ghcr-test:0.0.1"

		devDeploy(t, bundle)

		cmd := strings.Split("zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
		deployments, _, _ := e2e.UDS(cmd...)
		require.Contains(t, deployments, "podinfo")
		require.Contains(t, deployments, "nginx")

		remove(t, bundle)
	})

	t.Run("Test dev deploy with --set flag", func(t *testing.T) {
		bundleDir := "src/test/bundles/02-variables"
		bundleTarballPath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-variables-%s-0.0.1.tar.zst", e2e.Arch))
		_, stderr := runUDSCmd(t, "dev deploy "+bundleDir+" --set ANIMAL=Longhorns --set COUNTRY=Texas -l=debug")
		require.Contains(t, stderr, "This fun-fact was imported: Longhorns are the national animal of Texas")
		require.NotContains(t, stderr, "This fun-fact was imported: Unicorns are the national animal of Scotland")
		remove(t, bundleTarballPath)
	})

	t.Run("Test dev deploy with excluded components", func(t *testing.T) {
		// remove any lingering podinfo and nginx deployments
		e2e.UDS(strings.Split("zarf tools kubectl delete deploy -n podinfo podinfo", " ")...)
		e2e.UDS(strings.Split("zarf tools kubectl delete deploy -n nginx nginx-deployment", " ")...)

		bundleDir := "src/test/bundles/15-dev-deploy/optional"
		bundleName := "dev-deploy-exclude"
		bundleTarballPath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch))
		// exclude nginx from podinfo-nginx pkg
		runUDSCmd(t, "dev deploy "+bundleDir+" --exclude-components=podinfo-nginx.nginx")

		// inspect created bundle and ensure nginx isn't included
		// todo: refactor when we move 'inspect' to stdout and add --no-color flag
		_, stderr := runUDSCmd(t, "inspect "+bundleTarballPath)
		require.Contains(t, stderr, "-\x1b[95m podinfo\x1b[0m")
		require.NotContains(t, stderr, "-\x1b[95m nginx\x1b[0m")

		// ensure podinfo is deployed and nginx is not
		cmd := strings.Split("zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
		deployments, _, _ := e2e.UDS(cmd...)
		require.NotContains(t, deployments, "nginx")
		require.Contains(t, deployments, "podinfo")

		remove(t, bundleTarballPath)
	})
}
