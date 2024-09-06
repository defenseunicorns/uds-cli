// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackagesFlag(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)
	bundleDir := "src/test/bundles/03-local-and-remote"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

	runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, e2e.Arch))

	cmd := "zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'"
	t.Run("Test only podinfo deploy (local pkg)", func(t *testing.T) {
		runCmd(t, fmt.Sprintf("deploy %s --confirm -l=debug --packages %s", bundlePath, "podinfo"))
		deployments, _ := runCmd(t, cmd)
		require.Contains(t, deployments, "podinfo")
		require.NotContains(t, deployments, "nginx")

		runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundlePath))

		deployments, _ = runCmd(t, cmd)
		require.NotContains(t, deployments, "podinfo")
	})

	t.Run("Test only nginx deploy and remove (remote pkg)", func(t *testing.T) {
		runCmd(t, fmt.Sprintf("deploy %s --confirm -l=debug --packages %s", bundlePath, "nginx"))
		deployments, _ := runCmd(t, cmd)

		require.Contains(t, deployments, "nginx")
		require.NotContains(t, deployments, "podinfo")

		runCmd(t, fmt.Sprintf("remove %s --confirm --insecure --packages %s", bundlePath, "nginx"))
		deployments, _ = runCmd(t, cmd)
		require.NotContains(t, deployments, "nginx")
	})

	t.Run("Test both podinfo and nginx deploy and remove", func(t *testing.T) {
		runCmd(t, fmt.Sprintf("deploy %s --confirm -l=debug --packages %s", bundlePath, "podinfo,nginx"))
		deployments, _ := runCmd(t, cmd)
		require.Contains(t, deployments, "podinfo")
		require.Contains(t, deployments, "nginx")

		runCmd(t, fmt.Sprintf("remove %s --confirm --insecure --packages %s", bundlePath, "podinfo,nginx"))
		deployments, _ = runCmd(t, cmd)
		require.NotContains(t, deployments, "podinfo")
		require.NotContains(t, deployments, "nginx")
	})

	t.Run("Test invalid package deploy", func(t *testing.T) {
		_, stderr, _ := runCmdWithErr(fmt.Sprintf("deploy %s --confirm -l=debug --packages %s", bundlePath, "podinfo,nginx,peanuts"))
		require.Contains(t, stderr, "invalid zarf packages specified by --packages")
	})

	t.Run("Test invalid package remove", func(t *testing.T) {
		_, stderr, _ := runCmdWithErr(fmt.Sprintf("remove %s --confirm --insecure --packages %s", bundlePath, "podinfo,nginx,peanuts"))
		require.Contains(t, stderr, "invalid zarf packages specified by --packages")
	})
}

func TestResumeFlag(t *testing.T) {
	// delete nginx, podinfo, and uds (state) namespaces if they exist
	runCmdWithErr("zarf tools kubectl delete ns nginx podinfo")
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)
	bundleDir := "src/test/bundles/03-local-and-remote"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

	runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, e2e.Arch))

	bundleName := "test-local-and-remote"
	inspectLocal(t, bundlePath, bundleName)
	inspectLocalAndSBOMExtract(t, bundleName, bundlePath)

	getDeploymentsCmd := "zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'"

	// Deploy only podinfo (local pkg)
	runCmd(t, fmt.Sprintf("deploy %s --confirm -l=debug --packages %s", bundlePath, "podinfo"))
	deployments, _ := runCmd(t, getDeploymentsCmd)
	require.Contains(t, deployments, "podinfo")
	require.NotContains(t, deployments, "nginx")

	// Deploy bundle --resume (resumes remote pkg)
	runCmd(t, fmt.Sprintf("deploy %s --confirm -l=debug --resume", bundlePath))
	deployments, _ = runCmd(t, getDeploymentsCmd)
	require.Contains(t, deployments, "podinfo")
	require.Contains(t, deployments, "nginx")

	// Remove only podinfo
	runCmd(t, fmt.Sprintf("remove %s --confirm --insecure --packages %s", bundlePath, "podinfo"))
	deployments, _ = runCmd(t, getDeploymentsCmd)
	require.NotContains(t, deployments, "podinfo")
	require.Contains(t, deployments, "nginx")

	// Deploy only nginx (remote pkg)
	runCmd(t, fmt.Sprintf("deploy %s --confirm -l=debug --packages %s", bundlePath, "nginx"))
	deployments, _ = runCmd(t, getDeploymentsCmd)
	require.Contains(t, deployments, "nginx")
	require.NotContains(t, deployments, "podinfo")

	// Deploy bundle --resume (resumes remote pkg)
	runCmd(t, fmt.Sprintf("deploy %s --confirm -l=debug --resume", bundlePath))
	deployments, _ = runCmd(t, getDeploymentsCmd)
	require.Contains(t, deployments, "podinfo")
	require.Contains(t, deployments, "nginx")

	// Remove only nginx
	runCmd(t, fmt.Sprintf("remove %s --confirm --packages %s", bundlePath, "nginx"))
	deployments, _ = runCmd(t, getDeploymentsCmd)
	require.NotContains(t, deployments, "nginx")
	require.Contains(t, deployments, "podinfo")

	// Remove bundle
	runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundlePath))
	deployments, _ = runCmd(t, getDeploymentsCmd)
	require.NotContains(t, deployments, "podinfo")
	require.NotContains(t, deployments, "nginx")
}
