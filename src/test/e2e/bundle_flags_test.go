package test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackagesFlag(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)
	bundleDir := "src/test/bundles/03-local-and-remote"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

	createLocal(t, bundleDir, e2e.Arch)

	cmd := strings.Split("zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
	t.Run("Test only podinfo deploy (local pkg)", func(t *testing.T) {
		deployPackagesFlag(bundlePath, "podinfo")
		deployments, _, _ := e2e.UDS(cmd...)
		require.Contains(t, deployments, "podinfo")
		require.NotContains(t, deployments, "nginx")

		remove(t, bundlePath)
		deployments, _, _ = e2e.UDS(cmd...)
		require.NotContains(t, deployments, "podinfo")
	})

	t.Run("Test only nginx deploy and remove (remote pkg)", func(t *testing.T) {
		deployPackagesFlag(bundlePath, "nginx")
		deployments, _, _ := e2e.UDS(cmd...)
		require.Contains(t, deployments, "nginx")
		require.NotContains(t, deployments, "podinfo")
		remove(t, bundlePath)

		removePackagesFlag(bundlePath, "nginx")
		deployments, _, _ = e2e.UDS(cmd...)
		require.NotContains(t, deployments, "nginx")
	})

	t.Run("Test both podinfo and nginx deploy and remove", func(t *testing.T) {
		deployPackagesFlag(bundlePath, "podinfo,nginx")
		deployments, _, _ := e2e.UDS(cmd...)
		require.Contains(t, deployments, "podinfo")
		require.Contains(t, deployments, "nginx")

		removePackagesFlag(bundlePath, "podinfo,nginx")
		deployments, _, _ = e2e.UDS(cmd...)
		require.NotContains(t, deployments, "podinfo")
		require.NotContains(t, deployments, "nginx")
	})

	t.Run("Test invalid package deploy", func(t *testing.T) {
		_, stderr := deployPackagesFlag(bundlePath, "podinfo,nginx,peanuts")
		require.Contains(t, stderr, "invalid zarf packages specified by --packages")
	})

	t.Run("Test invalid package remove", func(t *testing.T) {
		_, stderr := removePackagesFlag(bundlePath, "podinfo,nginx,peanuts")
		require.Contains(t, stderr, "invalid zarf packages specified by --packages")
	})
}

func TestResumeFlag(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)
	bundleDir := "src/test/bundles/03-local-and-remote"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

	createLocal(t, bundleDir, e2e.Arch)
	inspectLocal(t, bundlePath)
	inspectLocalAndSBOMExtract(t, bundlePath)

	getDeploymentsCmd := strings.Split("zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")

	// Deploy only podinfo (local pkg)
	deployPackagesFlag(bundlePath, "podinfo")
	deployments, _, _ := e2e.UDS(getDeploymentsCmd...)
	require.Contains(t, deployments, "podinfo")
	require.NotContains(t, deployments, "nginx")

	// Deploy bundle --resume (resumes remote pkg)
	deployResumeFlag(t, bundlePath)
	deployments, _, _ = e2e.UDS(getDeploymentsCmd...)
	require.Contains(t, deployments, "podinfo")
	require.Contains(t, deployments, "nginx")

	// Remove only podinfo
	removePackagesFlag(bundlePath, "podinfo")
	deployments, _, _ = e2e.UDS(getDeploymentsCmd...)
	require.NotContains(t, deployments, "podinfo")
	require.Contains(t, deployments, "nginx")

	// Deploy only nginx (remote pkg)
	deployPackagesFlag(bundlePath, "nginx")
	deployments, _, _ = e2e.UDS(getDeploymentsCmd...)
	require.Contains(t, deployments, "nginx")
	require.NotContains(t, deployments, "podinfo")

	// Deploy bundle --resume (resumes remote pkg)
	deployResumeFlag(t, bundlePath)
	deployments, _, _ = e2e.UDS(getDeploymentsCmd...)
	require.Contains(t, deployments, "podinfo")
	require.Contains(t, deployments, "nginx")

	// Remove only nginx
	removePackagesFlag(bundlePath, "nginx")
	deployments, _, _ = e2e.UDS(getDeploymentsCmd...)
	require.NotContains(t, deployments, "nginx")
	require.Contains(t, deployments, "podinfo")

	// Remove bundle
	remove(t, bundlePath)
	deployments, _, _ = e2e.UDS(getDeploymentsCmd...)
	require.NotContains(t, deployments, "podinfo")
	require.NotContains(t, deployments, "nginx")
}
