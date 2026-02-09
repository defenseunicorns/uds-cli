// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test provides e2e tests for UDS.
package test

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBundleVariables(t *testing.T) {
	e2e.CreateZarfPkg(t, "src/test/packages/no-cluster/output-var", false)
	e2e.CreateZarfPkg(t, "src/test/packages/no-cluster/receive-var", false)
	os.Setenv("UDS_ANIMAL", "Unicorns")
	os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/02-variables", "uds-config.yaml"))

	t.Run("simple vars and global export", func(t *testing.T) {
		bundleDir := "src/test/bundles/02-variables"
		bundleTarballPath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-variables-%s-0.0.1.tar.zst", e2e.Arch))
		_, stderr := runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, e2e.Arch))
		require.Contains(t, stderr, "failed strict unmarshalling")
		_, stderr = runCmd(t, fmt.Sprintf("deploy %s --retries 1 --confirm", bundleTarballPath))
		bundleVariablesTestChecks(t, stderr, bundleTarballPath)
	})

	t.Run("bad var name in import", func(t *testing.T) {
		bundleDir := "src/test/bundles/02-variables/bad-var-name"
		_, stderr, _ := runCmdWithErr(fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, e2e.Arch))
		require.Contains(t, stderr, "does not have a matching export")
	})

	t.Run("var name collision with exported vars", func(t *testing.T) {
		e2e.CreateZarfPkg(t, "src/test/packages/no-cluster/output-var-collision", false)
		bundleDir := "src/test/bundles/02-variables/export-name-collision"
		bundleTarballPath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-export-name-collision-%s-0.0.1.tar.zst", e2e.Arch))
		runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, e2e.Arch))
		_, stderr := runCmd(t, fmt.Sprintf("deploy %s --retries 1 --confirm", bundleTarballPath))
		require.Contains(t, stderr, "This fun-fact was imported: Daffodils are the national flower of Wales")
		require.NotContains(t, stderr, "This fun-fact was imported: Unicorns are the national animal of Scotland")
	})
}

func bundleVariablesTestChecks(t *testing.T, stderr string, bundleTarballPath string) {
	require.NotContains(t, stderr, "CLIVersion is set to 'unset' which can cause issues with package creation and deployment")
	require.Contains(t, stderr, "This fun-fact was imported: Unicorns are the national animal of Scotland")
	require.Contains(t, stderr, "This fun-fact demonstrates precedence: The Red Dragon is the national symbol of Wales")
	require.Contains(t, stderr, "shared var in output-var pkg: burning.boats")
	require.Contains(t, stderr, "shared var in receive-var pkg: burning.boats")

	_, stderr = runCmd(t, "deploy "+bundleTarballPath+" --set ANIMAL=Longhorns --set COUNTRY=Texas --confirm -l=debug")
	require.Contains(t, stderr, "This fun-fact was imported: Longhorns are the national animal of Texas")
	require.NotContains(t, stderr, "This fun-fact was imported: Unicorns are the national animal of Scotland")

	_, stderr = runCmd(t, "deploy "+bundleTarballPath+" --set output-var.SPECIFIC_PKG_VAR=output-var-set --confirm -l=debug")
	require.Contains(t, stderr, "output-var SPECIFIC_PKG_VAR = output-var-set")
	require.Contains(t, stderr, "receive-var SPECIFIC_PKG_VAR = not-set")

	_, stderr = runCmd(t, "deploy "+bundleTarballPath+" --set output-var.specific_pkg_var=output --set receive-var.SPECIFIC_PKG_VAR=receive --confirm -l=debug")
	require.Contains(t, stderr, "output-var SPECIFIC_PKG_VAR = output")
	require.Contains(t, stderr, "receive-var SPECIFIC_PKG_VAR = receive")

	_, stderr = runCmd(t, "deploy "+bundleTarballPath+" --set SPECIFIC_PKG_VAR=errbody --confirm -l=debug")
	require.Contains(t, stderr, "output-var SPECIFIC_PKG_VAR = errbody")
	require.Contains(t, stderr, "receive-var SPECIFIC_PKG_VAR = errbody")
}

func TestBundleWithHelmOverrides(t *testing.T) {
	deployZarfInit(t)
	e2e.HelmDepUpdate(t, "src/test/packages/helm/unicorn-podinfo")
	e2e.CreateZarfPkg(t, "src/test/packages/helm", false)
	bundleDir := "src/test/bundles/07-helm-overrides"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-helm-overrides-%s-0.0.1.tar.zst", e2e.Arch))
	err := os.Setenv("UDS_CONFIG", filepath.Join(bundleDir, "uds-config.yaml"))
	require.NoError(t, err)

	runCmd(t, fmt.Sprintf("create %s --confirm --insecure -a %s", bundleDir, e2e.Arch))
	runCmd(t, fmt.Sprintf("deploy %s --retries 1 --confirm", bundlePath))

	// test values overrides
	t.Run("check values overrides", func(t *testing.T) {
		outputNumReplicas, _ := runCmd(t, "zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.replicas}'")
		require.Equal(t, "'2'", outputNumReplicas)
	})

	t.Run("check object-type override in values", func(t *testing.T) {
		annotations, _ := runCmd(t, "zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.metadata.annotations}'")
		require.Contains(t, annotations, "\"customAnnotation\":\"customValue\"")
	})

	t.Run("check list-type override in values", func(t *testing.T) {
		tolerations, _ := runCmd(t, "zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.tolerations}'")
		require.Contains(t, tolerations, "\"key\":\"uds\"")
		require.Contains(t, tolerations, "\"value\":\"defense\"")
		require.Contains(t, tolerations, "\"key\":\"unicorn\"")
		require.Contains(t, tolerations, "\"effect\":\"NoSchedule\"")
	})

	// test variables overrides
	t.Run("check variables overrides, use default", func(t *testing.T) {
		podCmd, _ := runCmd(t, "zarf tools kubectl get deploy unicorn-podinfo -n podinfo -o=jsonpath='{.spec.template.spec.containers[*].command[*]}'")
		require.Contains(t, podCmd, "--level=debug")
	})

	t.Run("check variables overrides, default overwritten by config", func(t *testing.T) {
		outputUIColor, _ := runCmd(t, "zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'")
		require.Equal(t, "'green, yellow'", outputUIColor)
	})

	t.Run("check variables overrides, no default but set in config", func(t *testing.T) {
		outputMsg, _ := runCmd(t, "zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_MESSAGE\")].value}'")
		require.Equal(t, "'Hello Unicorn'", outputMsg)
	})

	t.Run("check variables overrides, no default and not set in config", func(t *testing.T) {
		secretValue, _ := runCmd(t, "zarf tools kubectl get secret test-secret -n podinfo -o jsonpath=\"{.data.test}\"")
		// expect the value to be from the underlying chart's values.yaml, no overrides
		require.Equal(t, "\"dGVzdC1zZWNyZXQ=\"", secretValue)
	})

	t.Run("check variables overrides with an object-type value", func(t *testing.T) {
		securityContext, _ := runCmd(t, "zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].securityContext}'")
		require.Contains(t, securityContext, "NET_ADMIN")
		require.Contains(t, securityContext, "\"runAsGroup\":4000")
	})

	t.Run("check variables overrides with a list-type value", func(t *testing.T) {
		hosts, _ := runCmd(t, "zarf tools kubectl get ingress -n podinfo unicorn-podinfo -o=jsonpath='{.spec.rules[*].host}''")
		require.Contains(t, hosts, "podinfo.burning.boats")
		require.Contains(t, hosts, "podinfo.unicorns")
	})

	t.Run("check variables overrides with a file type value", func(t *testing.T) {
		stdout, _ := runCmd(t, "zarf tools kubectl get secret -n podinfo test-file-secret -o=jsonpath={.data.test}")
		decoded, err := base64.StdEncoding.DecodeString(stdout)
		require.NoError(t, err)
		require.Contains(t, string(decoded), "ssh-rsa")
	})

	t.Run("check multiple charts under same component deploy", func(t *testing.T) {
		stdout, _ := runCmd(t, "zarf tools kubectl get secret -n second-chart second-chart-secret -o=jsonpath={.data.test}")
		decoded, err := base64.StdEncoding.DecodeString(stdout)
		require.NoError(t, err)
		require.Contains(t, string(decoded), "ssh-rsa")
	})

	runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundlePath))
}

func TestBundleWithHelmOverridesValuesFile(t *testing.T) {
	deployZarfInit(t)
	e2e.HelmDepUpdate(t, "src/test/packages/helm/unicorn-podinfo")
	e2e.CreateZarfPkg(t, "src/test/packages/helm", false)
	bundleDir := "src/test/bundles/07-helm-overrides/values-file"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-helm-values-file-%s-0.0.1.tar.zst", e2e.Arch))
	err := os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/07-helm-overrides", "uds-config.yaml"))
	require.NoError(t, err)

	runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, e2e.Arch))
	runCmd(t, fmt.Sprintf("deploy %s --retries 1 --confirm", bundlePath))

	// test values overrides
	t.Run("check values overrides", func(t *testing.T) {
		outputNumReplicas, _ := runCmd(t, "zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.replicas}'")
		require.Equal(t, "'2'", outputNumReplicas)
	})

	t.Run("check object-type override in values", func(t *testing.T) {
		annotations, _ := runCmd(t, "zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.metadata.annotations}'")
		require.Contains(t, annotations, "\"customAnnotation\":\"customValue2\"")
	})

	t.Run("check list-type override in values", func(t *testing.T) {
		tolerations, _ := runCmd(t, "zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.tolerations}'")
		require.Contains(t, tolerations, "\"key\":\"uds\"")
		require.Contains(t, tolerations, "\"value\":\"defense\"")
		require.Contains(t, tolerations, "\"key\":\"unicorn\"")
		require.Contains(t, tolerations, "\"effect\":\"NoSchedule\"")
	})
}

func TestBundleWithDupPkgs(t *testing.T) {
	deployZarfInit(t)
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	zarfPkgPath := "src/test/packages/helm"
	e2e.HelmDepUpdate(t, fmt.Sprintf("%s/unicorn-podinfo", zarfPkgPath))
	e2e.CreateZarfPkg(t, zarfPkgPath, false)
	name := "duplicates"
	pkg := filepath.Join(zarfPkgPath, fmt.Sprintf("zarf-package-helm-overrides-%s-0.0.1.tar.zst", e2e.Arch))
<<<<<<< Updated upstream
	runCmd(t, fmt.Sprintf("zarf package publish %s oci://localhost:888 --plain-http --oci-concurrency=10 -l debug --no-progress", pkg))
=======
	runCmd(t, fmt.Sprintf("zarf package publish %s oci://localhost:888 --insecure --oci-concurrency=10 -l debug", pkg))
>>>>>>> Stashed changes
	bundleDir := "src/test/bundles/07-helm-overrides/duplicate"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", name, e2e.Arch))

	runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, e2e.Arch))

	// remove namespace after tests
	defer func() {
		runCmd(t, "zarf tools kubectl delete ns override-ns another-override-ns")
	}()

	// helper fn to check the different namespaces for the deployment
	checkDeployments := func(t *testing.T) {
		for _, ns := range []string{"override-ns", "another-override-ns"} {
			deployment, _ := runCmd(t, fmt.Sprintf("zarf tools kubectl get deploy -n %s -o=jsonpath='{.items[*].metadata.name}'", ns))
			require.Equal(t, "'unicorn-podinfo'", deployment)
		}
	}

	t.Run("test namespace override + dup pkgs in local bundle", func(t *testing.T) {
		runCmd(t, fmt.Sprintf("deploy %s --retries 1 --confirm", bundlePath))
		checkDeployments(t)
		runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundlePath))
	})

	t.Run("test namespace override + dup pkgs in remote bundle", func(t *testing.T) {
		ref := fmt.Sprintf("localhost:888/%s:0.0.1", name)
		runCmd(t, fmt.Sprintf("publish %s localhost:888 --insecure", bundlePath))
		runCmd(t, fmt.Sprintf("deploy %s --insecure --confirm", ref))
		checkDeployments(t)
		runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", ref))
	})
}

func TestBundleWithEnvVarHelmOverrides(t *testing.T) {
	// set up configs and env vars
	deployZarfInit(t)
	e2e.HelmDepUpdate(t, "src/test/packages/helm/unicorn-podinfo")
	e2e.CreateZarfPkg(t, "src/test/packages/helm", false)
	color := "purple"
	b64Secret := "dGhhdCBhaW50IG15IHRydWNrCg=="
	err := os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/07-helm-overrides", "uds-config.yaml"))
	require.NoError(t, err)
	err = os.Setenv("UDS_UI_COLOR", color)
	require.NoError(t, err)
	err = os.Setenv("UDS_UI_MSG", "im set by an env var")
	require.NoError(t, err)
	err = os.Setenv("UDS_SECRET_VAL", b64Secret)
	require.NoError(t, err)

	// create and deploy bundle
	bundleDir := "src/test/bundles/07-helm-overrides"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-helm-overrides-%s-0.0.1.tar.zst", e2e.Arch))
	runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, e2e.Arch))
	runCmd(t, fmt.Sprintf("deploy %s --retries 1 --confirm", bundlePath))

	t.Run("check override variables, ensure they are coming from env vars and take highest precedence", func(t *testing.T) {
		outputUIColor, _ := runCmd(t, "z tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'")
		require.Equal(t, fmt.Sprintf("'%s'", color), outputUIColor)
	})

	t.Run("check override secret val", func(t *testing.T) {
		secretValue, _ := runCmd(t, "z tools kubectl get secret test-secret -n podinfo -o jsonpath=\"{.data.test}\"")
		require.Equal(t, fmt.Sprintf("\"%s\"", b64Secret), secretValue)
	})

	t.Run("ensure --set overrides take precedence over env vars", func(t *testing.T) {
		runCmd(t, fmt.Sprintf("deploy %s --set UI_COLOR=orange --set helm-overrides.ui_msg=foo --confirm", bundlePath))

		outputUIColor, _ := runCmd(t, "z tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'")
		require.Equal(t, "'orange'", outputUIColor)

		outputMsg, _ := runCmd(t, "z tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_MESSAGE\")].value}'")
		require.Equal(t, "'foo'", outputMsg)
	})

	runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundlePath))
}

func TestVariablePrecedence(t *testing.T) {
	// precedence rules: env var > uds-config.variables > uds-config.shared > default
	deployZarfInit(t)
	e2e.HelmDepUpdate(t, "src/test/packages/helm/unicorn-podinfo")
	e2e.CreateZarfPkg(t, "src/test/packages/helm", false)
	e2e.CreateZarfPkg(t, "src/test/packages/no-cluster/output-var", false)
	bundleDir := "src/test/bundles/08-var-precedence"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-var-precedence-%s-0.0.1.tar.zst", e2e.Arch))
	err := os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/08-var-precedence", "uds-config.yaml"))
	require.NoError(t, err)
	runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, e2e.Arch))

	color := "green"
	err = os.Setenv("UDS_UI_COLOR", color)
	require.NoError(t, err)
	_, stderr := runCmd(t, fmt.Sprintf("deploy %s --retries 1 --confirm", bundlePath))

	t.Run("test precedence, env var > uds-config.variables > uds-config.shared", func(t *testing.T) {
		// test env har taking highest precedence
		outputUIColor, _ := runCmd(t, "zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'")
		require.Equal(t, fmt.Sprintf("'%s'", color), outputUIColor)

		// test uds-config.variables overriding a shared var
		require.Contains(t, stderr, "shared var in output-var pkg: unicorns.uds.dev")

		// test uds-config.shared overriding a Zarf var
		require.Contains(t, stderr, "shared var in helm-overrides pkg: burning.boats")
	})

	t.Run("test uds-config.shared overriding values in a Helm chart (ie. bundle overrides)", func(t *testing.T) {
		backend, _ := runCmd(t, "zarf tools kubectl get deploy unicorn-podinfo -n podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_BACKEND_URL\")].value}'")
		require.Equal(t, fmt.Sprintf("'%s'", "burning.boats"), backend)
	})

	runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundlePath))
}

func TestExportVarsAsGlobalVars(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/no-cluster/output-var", false)
	e2e.HelmDepUpdate(t, "src/test/packages/helm/unicorn-podinfo")
	e2e.CreateZarfPkg(t, "src/test/packages/helm", false)
	bundleDir := "src/test/bundles/12-exported-pkg-vars"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-export-vars-%s-0.0.1.tar.zst", e2e.Arch))

	runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, e2e.Arch))
	runCmd(t, fmt.Sprintf("deploy %s --retries 1 --confirm", bundlePath))

	t.Run("check templated variables overrides in values", func(t *testing.T) {
		outputUIColor, _ := runCmd(t, "zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'")
		require.Equal(t, "'orange'", outputUIColor)
	})

	t.Run("check multiple templated variables as object overrides in values", func(t *testing.T) {
		annotations, _ := runCmd(t, "zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.metadata.annotations}'")
		require.Contains(t, annotations, "\"customAnnotation\":\"orangeAnnotation\"")
	})

	t.Run("check templated variable list-type overrides in values", func(t *testing.T) {
		tolerations, _ := runCmd(t, "zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.tolerations}'")
		require.Contains(t, tolerations, "\"key\":\"uds\"")
		require.Contains(t, tolerations, "\"value\":\"true\"")
		require.Contains(t, tolerations, "\"key\":\"unicorn\"")
		require.Contains(t, tolerations, "\"value\":\"defense\"")
	})

	runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundlePath))
}
