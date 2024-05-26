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

	t.Run("Test dev deploy with ref flag", func(t *testing.T) {

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

		bundleDir := "src/test/bundles/03-local-and-remote"

		cmd := strings.Split(fmt.Sprintf("dev deploy %s --flavor %s", bundleDir, "podinfo=three"), " ")
		_, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl get deployment -n podinfo-flavor podinfo -o=jsonpath='{.spec.template.spec.containers[0].image}'", " ")
		ref, _, err := e2e.UDS(cmd...)
		require.Contains(t, ref, "ghcr.io/stefanprodan/podinfo:6.6.3")
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl delete ns podinfo nginx zarf podinfo-flavor", " ")
		_, _, err = e2e.UDS(cmd...)
		require.NoError(t, err)
	})
	t.Run("Test dev deploy with global flavor", func(t *testing.T) {

		bundleDir := "src/test/bundles/03-local-and-remote"

		cmd := strings.Split(fmt.Sprintf("dev deploy %s --flavor-all %s", bundleDir, "three"), " ")
		_, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl get deployment -n podinfo-flavor podinfo -o=jsonpath='{.spec.template.spec.containers[0].image}'", " ")
		ref, _, err := e2e.UDS(cmd...)
		require.Contains(t, ref, "ghcr.io/stefanprodan/podinfo:6.6.3")
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl delete ns podinfo nginx zarf podinfo-flavor", " ")
		_, _, err = e2e.UDS(cmd...)
		require.NoError(t, err)
	})

	t.Run("Test dev deploy with flavor and force create", func(t *testing.T) {

		bundleDir := "src/test/bundles/03-local-and-remote"

		// create flavor three podinfo-flavor package
		cmd = strings.Split("zarf package create src/test/packages/podinfo --flavor three", " ")

		// dev deploy with flavor two and --force-create
		cmd := strings.Split(fmt.Sprintf("dev deploy %s --flavor %s --force-create", bundleDir, "podinfo=two"), " ")
		_, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl get deployment -n podinfo-flavor podinfo -o=jsonpath='{.spec.template.spec.containers[0].image}'", " ")
		ref, _, err := e2e.UDS(cmd...)
		// assert that podinfo package with flavor two was deployed.
		require.Contains(t, ref, "ghcr.io/stefanprodan/podinfo:6.6.2")
		require.NoError(t, err)

		cmd = strings.Split("zarf tools kubectl delete ns podinfo nginx zarf podinfo-flavor", " ")
		_, _, err = e2e.UDS(cmd...)
		require.NoError(t, err)
	})
}
