// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackagesFlag(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "testdata/legacy/packages/podinfo", false)
	bundleDir := "testdata/legacy/bundles/03-local-and-remote"
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
	// delete nginx, podinfo, and uds namespaces if they exist
	runCmdWithErr("zarf tools kubectl delete ns nginx podinfo") // intentionally not err checking this cmd
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "testdata/legacy/packages/podinfo", false)
	bundleDir := "testdata/legacy/bundles/03-local-and-remote"
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

func TestResumeFlagWithPackageNamespaceOverrideDuplicates(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "testdata/legacy/packages/nginx-namespace-override", false)
	seedBundleDir := "testdata/legacy/bundles/07-helm-overrides/package-namespace-resume-seed"
	seedBundlePath := filepath.Join(seedBundleDir, fmt.Sprintf("uds-bundle-package-namespace-resume-seed-%s-0.0.1.tar.zst", e2e.Arch))
	bundleDir := "testdata/legacy/bundles/07-helm-overrides/package-namespace-resume"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-package-namespace-resume-%s-0.0.1.tar.zst", e2e.Arch))
	waitForDeployment := func(namespace, condition, timeout string) (string, string, error) {
		return runCmdWithErr(fmt.Sprintf("zarf tools wait-for resource deployment nginx-deployment %s -n %s --timeout %s", condition, namespace, timeout))
	}

	runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", seedBundleDir, e2e.Arch))
	runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, e2e.Arch))
	defer func() {
		_, _, _ = runCmdWithErr(fmt.Sprintf("remove %s --confirm --insecure", bundlePath))
		_, _, _ = runCmdWithErr(fmt.Sprintf("remove %s --confirm --insecure", seedBundlePath))
	}()

	runCmd(t, fmt.Sprintf("deploy %s --confirm", seedBundlePath))
	_, _, err := waitForDeployment("resume-override-a", "available", "30s")
	require.NoError(t, err)

	runCmd(t, fmt.Sprintf("deploy %s --confirm --resume", bundlePath))
	_, _, err = waitForDeployment("resume-override-b", "available", "30s")
	require.NoError(t, err)

	runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundlePath))
	_, _, err = waitForDeployment("resume-override-a", "delete", "30s")
	require.NoError(t, err)
	_, _, err = waitForDeployment("resume-override-b", "delete", "30s")
	require.NoError(t, err)
}

func TestResumeFlagInterruptedUpgrade(t *testing.T) {
	deployZarfInit(t)
	root := t.TempDir()
	names := []string{"resume-upgrade-first", "resume-upgrade-second", "resume-upgrade-third"}
	logs := make(map[string]string, len(names))
	for _, name := range names {
		logs[name] = filepath.Join(root, name+".log")
	}
	writeFile := func(path, contents string) {
		t.Helper()
		require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	}
	createBundle := func(version string) string {
		t.Helper()
		dir := filepath.Join(root, version)
		require.NoError(t, os.MkdirAll(dir, 0o700))
		bundleYAML := fmt.Sprintf("kind: UDSBundle\nmetadata:\n  name: resume-upgrade\n  version: %s\npackages:\n", version)
		for _, name := range names {
			pkgDir := filepath.Join(dir, name)
			require.NoError(t, os.MkdirAll(pkgDir, 0o700))
			writeFile(filepath.Join(pkgDir, "configmap.yaml"), fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
data:
  version: %q
`, name, version))
			writeFile(filepath.Join(pkgDir, "zarf.yaml"), fmt.Sprintf(`kind: ZarfPackageConfig
metadata:
  name: %s
  version: %s
components:
  - name: main
    required: true
    manifests:
      - name: version
        namespace: resume-upgrade
        files:
          - configmap.yaml
    actions:
      onDeploy:
        after:
          - cmd: echo %s >> %s
`, name, version, version, logs[name]))
			runCmd(t, fmt.Sprintf("zarf package create %s --confirm --skip-sbom -a %s -o %s", pkgDir, e2e.Arch, pkgDir))
			bundleYAML += fmt.Sprintf("  - name: %s\n    path: %s\n    ref: %s\n", name, pkgDir, version)
		}
		writeFile(filepath.Join(dir, "uds-bundle.yaml"), bundleYAML)
		runCmd(t, fmt.Sprintf("create %s --confirm -a %s", dir, e2e.Arch))
		return filepath.Join(dir, fmt.Sprintf("uds-bundle-resume-upgrade-%s-%s.tar.zst", e2e.Arch, version))
	}
	oldBundle := createBundle("0.0.1")
	newBundle := createBundle("0.0.2")
	t.Cleanup(func() {
		runCmd(t, fmt.Sprintf("remove %s --confirm", newBundle))
	})

	runCmd(t, fmt.Sprintf("deploy %s --confirm", oldBundle))
	// This produces the state left by an interruption between packages.
	runCmd(t, fmt.Sprintf("deploy %s --confirm --packages %s", newBundle, names[0]))
	runCmd(t, fmt.Sprintf("deploy %s --confirm --resume", newBundle))

	checkUpgrade := func() {
		t.Helper()
		for _, name := range names {
			version, _ := runCmd(t, fmt.Sprintf("zarf tools kubectl get configmap %s -n resume-upgrade -o=jsonpath={.data.version}", name))
			require.Equal(t, "0.0.2", version)
			deployments, err := os.ReadFile(logs[name])
			require.NoError(t, err)
			require.Equal(t, "0.0.1\n0.0.2\n", string(deployments), "%s must deploy each version only once", name)
		}
	}
	checkUpgrade()
	runCmd(t, fmt.Sprintf("deploy %s --confirm --resume", newBundle))
	checkUpgrade()
}
