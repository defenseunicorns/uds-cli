// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		createLocal(t, bundleDir, e2e.Arch)
		_, stderr := deploy(t, bundleTarballPath)
		bundleVariablesTestChecks(t, stderr, bundleTarballPath)
	})

	t.Run("bad var name in import", func(t *testing.T) {
		bundleDir := "src/test/bundles/02-variables/bad-var-name"
		stderr := createLocalError(bundleDir, e2e.Arch)
		require.Contains(t, stderr, "does not have a matching export")
	})

	t.Run("var name collision with exported vars", func(t *testing.T) {
		e2e.CreateZarfPkg(t, "src/test/packages/no-cluster/output-var-collision", false)
		bundleDir := "src/test/bundles/02-variables/export-name-collision"
		bundleTarballPath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-export-name-collision-%s-0.0.1.tar.zst", e2e.Arch))
		createLocal(t, bundleDir, e2e.Arch)
		_, stderr := deploy(t, bundleTarballPath)
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
	err := os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/07-helm-overrides", "uds-config.yaml"))
	require.NoError(t, err)

	createLocal(t, bundleDir, e2e.Arch)
	deploy(t, bundlePath)

	// test values overrides
	t.Run("check values overrides", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.replicas}'", " ")
		outputNumReplicas, _, err := e2e.UDS(cmd...)
		require.Equal(t, "'2'", outputNumReplicas)
		require.NoError(t, err)
	})

	t.Run("check object-type override in values", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.metadata.annotations}'", " ")
		annotations, _, err := e2e.UDS(cmd...)
		require.Contains(t, annotations, "\"customAnnotation\":\"customValue\"")
		require.NoError(t, err)

	})

	t.Run("check list-type override in values", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.tolerations}'", " ")
		tolerations, _, err := e2e.UDS(cmd...)
		require.Contains(t, tolerations, "\"key\":\"uds\"")
		require.Contains(t, tolerations, "\"value\":\"defense\"")
		require.Contains(t, tolerations, "\"key\":\"unicorn\"")
		require.Contains(t, tolerations, "\"effect\":\"NoSchedule\"")
		require.NoError(t, err)

	})

	// test variables overrides
	t.Run("check variables overrides, use default", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deploy unicorn-podinfo -n podinfo -o=jsonpath='{.spec.template.spec.containers[*].command[*]}'", " ")
		podCmd, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)
		require.Contains(t, podCmd, "--level=debug")
	})

	t.Run("check variables overrides, default overwritten by config", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'", " ")
		outputUIColor, _, err := e2e.UDS(cmd...)
		require.Equal(t, "'green, yellow'", outputUIColor)
		require.NoError(t, err)
	})

	t.Run("check variables overrides, no default but set in config", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_MESSAGE\")].value}'", " ")
		outputMsg, _, err := e2e.UDS(cmd...)
		require.Equal(t, "'Hello Unicorn'", outputMsg)
		require.NoError(t, err)
	})

	t.Run("check variables overrides, no default and not set in config", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get secret test-secret -n podinfo -o jsonpath=\"{.data.test}\"", " ")
		secretValue, _, err := e2e.UDS(cmd...)
		// expect the value to be from the underlying chart's values.yaml, no overrides
		require.Equal(t, "\"dGVzdC1zZWNyZXQ=\"", secretValue)
		require.NoError(t, err)
	})

	t.Run("check variables overrides with an object-type value", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].securityContext}'", " ")
		securityContext, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)
		require.Contains(t, securityContext, "NET_ADMIN")
		require.Contains(t, securityContext, "\"runAsGroup\":4000")
	})

	t.Run("check variables overrides with a list-type value", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get ingress -n podinfo unicorn-podinfo -o=jsonpath='{.spec.rules[*].host}''", " ")
		hosts, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)
		require.Contains(t, hosts, "podinfo.burning.boats")
		require.Contains(t, hosts, "podinfo.unicorns")
	})

	remove(t, bundlePath)
}

func TestBundleWithHelmOverridesValuesFile(t *testing.T) {
	deployZarfInit(t)
	e2e.HelmDepUpdate(t, "src/test/packages/helm/unicorn-podinfo")
	e2e.CreateZarfPkg(t, "src/test/packages/helm", false)
	bundleDir := "src/test/bundles/07-helm-overrides/values-file"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-helm-values-file-%s-0.0.1.tar.zst", e2e.Arch))
	err := os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/07-helm-overrides", "uds-config.yaml"))
	require.NoError(t, err)

	createLocal(t, bundleDir, e2e.Arch)
	deploy(t, bundlePath)

	// test values overrides
	t.Run("check values overrides", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.replicas}'", " ")
		outputNumReplicas, _, err := e2e.UDS(cmd...)
		require.Equal(t, "'2'", outputNumReplicas)
		require.NoError(t, err)
	})

	t.Run("check object-type override in values", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.metadata.annotations}'", " ")
		annotations, _, err := e2e.UDS(cmd...)
		require.Contains(t, annotations, "\"customAnnotation\":\"customValue2\"")
		require.NoError(t, err)
	})

	t.Run("check list-type override in values", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.tolerations}'", " ")
		tolerations, _, err := e2e.UDS(cmd...)
		require.Contains(t, tolerations, "\"key\":\"uds\"")
		require.Contains(t, tolerations, "\"value\":\"defense\"")
		require.Contains(t, tolerations, "\"key\":\"unicorn\"")
		require.Contains(t, tolerations, "\"effect\":\"NoSchedule\"")
		require.NoError(t, err)

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
	zarfPublish(t, pkg, "localhost:888")
	bundleDir := "src/test/bundles/07-helm-overrides/duplicate"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", name, e2e.Arch))

	createLocal(t, bundleDir, e2e.Arch)

	// remove namespace after tests
	defer func() {
		cmd := strings.Split("zarf tools kubectl delete ns override-ns another-override-ns", " ")
		_, _, _ = e2e.UDS(cmd...)
	}()

	// helper fn to check the different namespaces for the deployment
	checkDeployments := func(t *testing.T) {
		for _, ns := range []string{"override-ns", "another-override-ns"} {
			cmd := strings.Split(fmt.Sprintf("zarf tools kubectl get deploy -n %s -o=jsonpath='{.items[*].metadata.name}'", ns), " ")
			deployment, _, _ := e2e.UDS(cmd...)
			require.Equal(t, "'unicorn-podinfo'", deployment)
		}
	}

	t.Run("test namespace override + dup pkgs in local bundle", func(t *testing.T) {
		deploy(t, bundlePath)
		checkDeployments(t)
		remove(t, bundlePath)
	})

	t.Run("test namespace override + dup pkgs in remote bundle", func(t *testing.T) {
		publishInsecure(t, bundlePath, "localhost:888")
		deployInsecure(t, fmt.Sprintf("localhost:888/%s:0.0.1", name))
		checkDeployments(t)
		removeInsecure(t, fmt.Sprintf("localhost:888/%s:0.0.1", name))
	})
}

func TestBundleWithEnvVarHelmOverrides(t *testing.T) {
	// set up configs and env vars
	deployZarfInit(t)
	e2e.HelmDepUpdate(t, "src/test/packages/helm/unicorn-podinfo")
	e2e.CreateZarfPkg(t, "src/test/packages/helm", false)
	color := "purple"
	b64Secret := "dGhhdCBhaW50IG15IHRydWNr"
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
	createLocal(t, bundleDir, e2e.Arch)
	deploy(t, bundlePath)

	t.Run("check override variables, ensure they are coming from env vars and take highest precedence", func(t *testing.T) {
		cmd := strings.Split("z tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'", " ")
		outputUIColor, _, err := e2e.UDS(cmd...)
		require.Equal(t, fmt.Sprintf("'%s'", color), outputUIColor)
		require.NoError(t, err)
	})

	t.Run("check override secret val", func(t *testing.T) {
		cmd := strings.Split("z tools kubectl get secret test-secret -n podinfo -o jsonpath=\"{.data.test}\"", " ")
		secretValue, _, err := e2e.UDS(cmd...)
		require.Equal(t, fmt.Sprintf("\"%s\"", b64Secret), secretValue)
		require.NoError(t, err)
	})

	t.Run("ensure --set overrides take precedence over env vars", func(t *testing.T) {
		deployCmd := fmt.Sprintf("deploy %s --set UI_COLOR=orange --set helm-overrides.ui_msg=foo --confirm", bundlePath)
		_, _, err := e2e.UDS(strings.Split(deployCmd, " ")...)
		require.NoError(t, err)

		cmd := strings.Split("z tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'", " ")
		outputUIColor, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)
		require.Equal(t, "'orange'", outputUIColor)

		cmd = strings.Split("z tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_MESSAGE\")].value}'", " ")
		outputMsg, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)
		require.Equal(t, "'foo'", outputMsg)
	})

	remove(t, bundlePath)
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
	createLocal(t, bundleDir, e2e.Arch)

	color := "green"
	err = os.Setenv("UDS_UI_COLOR", color)
	require.NoError(t, err)
	_, stderr := deploy(t, bundlePath)

	t.Run("test precedence, env var > uds-config.variables > uds-config.shared", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'", " ")
		// test env har taking highest precedence
		outputUIColor, _, err := e2e.UDS(cmd...)
		require.Equal(t, fmt.Sprintf("'%s'", color), outputUIColor)
		require.NoError(t, err)

		// test uds-config.variables overriding a shared var
		require.Contains(t, stderr, "shared var in output-var pkg: unicorns.uds.dev")

		// test uds-config.shared overriding a Zarf var
		require.Contains(t, stderr, "shared var in helm-overrides pkg: burning.boats")
	})

	t.Run("test uds-config.shared overriding values in a Helm chart (ie. bundle overrides)", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deploy unicorn-podinfo -n podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_BACKEND_URL\")].value}'", " ")
		backend, _, err := e2e.UDS(cmd...)
		require.Equal(t, fmt.Sprintf("'%s'", "burning.boats"), backend)
		require.NoError(t, err)
	})

	remove(t, bundlePath)
}

func TestExportVarsAsGlobalVars(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/no-cluster/output-var", false)
	e2e.HelmDepUpdate(t, "src/test/packages/helm/unicorn-podinfo")
	e2e.CreateZarfPkg(t, "src/test/packages/helm", false)
	bundleDir := "src/test/bundles/12-exported-pkg-vars"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-export-vars-%s-0.0.1.tar.zst", e2e.Arch))

	createLocal(t, bundleDir, e2e.Arch)
	deploy(t, bundlePath)

	t.Run("check templated variables overrides in values", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'", " ")
		outputUIColor, _, err := e2e.UDS(cmd...)
		require.Equal(t, "'orange'", outputUIColor)
		require.NoError(t, err)
	})

	t.Run("check multiple templated variables as object overrides in values", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.metadata.annotations}'", " ")
		annotations, _, err := e2e.UDS(cmd...)
		require.Contains(t, annotations, "\"customAnnotation\":\"orangeAnnotation\"")
		require.NoError(t, err)
	})

	t.Run("check templated variable list-type overrides in values", func(t *testing.T) {
		cmd := strings.Split("zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.tolerations}'", " ")
		tolerations, _, err := e2e.UDS(cmd...)
		require.Contains(t, tolerations, "\"key\":\"uds\"")
		require.Contains(t, tolerations, "\"value\":\"true\"")
		require.Contains(t, tolerations, "\"key\":\"unicorn\"")
		require.Contains(t, tolerations, "\"value\":\"defense\"")
		require.NoError(t, err)
	})

	remove(t, bundlePath)
}
