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
	zarfPkgPath1 := "src/test/packages/no-cluster/output-var"
	zarfPkgPath2 := "src/test/packages/no-cluster/receive-var"
	e2e.CreateZarfPkg(t, zarfPkgPath1, false)
	e2e.CreateZarfPkg(t, zarfPkgPath2, false)

	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	pkg := filepath.Join(zarfPkgPath1, fmt.Sprintf("zarf-package-output-var-%s-0.0.1.tar.zst", e2e.Arch))
	zarfPublish(t, pkg, "localhost:888")

	pkg = filepath.Join(zarfPkgPath2, fmt.Sprintf("zarf-package-receive-var-%s-0.0.1.tar.zst", e2e.Arch))
	zarfPublish(t, pkg, "localhost:888")

	bundleDir := "src/test/bundles/02-simple-vars"
	bundleTarballPath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-simple-vars-%s-0.0.1.tar.zst", e2e.Arch))

	createLocal(t, bundleDir, e2e.Arch)
	createRemoteInsecure(t, bundleDir, "localhost:888", e2e.Arch)

	os.Setenv("UDS_ANIMAL", "Unicorns")
	os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/02-simple-vars", "uds-config.yaml"))

	_, stderr := deploy(t, bundleTarballPath)

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

	// check values overrides
	cmd := strings.Split("zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.replicas}'", " ")
	outputNumReplicas, _, err := e2e.UDS(cmd...)
	require.Equal(t, "'2'", outputNumReplicas)
	require.NoError(t, err)

	// check object-type override in values
	cmd = strings.Split("zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.metadata.annotations}'", " ")
	annotations, _, err := e2e.UDS(cmd...)
	require.Contains(t, annotations, "\"customAnnotation\":\"customValue\"")
	require.NoError(t, err)

	// check list-type override in values
	cmd = strings.Split("zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.tolerations}'", " ")
	tolerations, _, err := e2e.UDS(cmd...)
	require.Contains(t, tolerations, "\"key\":\"uds\"")
	require.Contains(t, tolerations, "\"value\":\"defense\"")
	require.Contains(t, tolerations, "\"key\":\"unicorn\"")
	require.Contains(t, tolerations, "\"effect\":\"NoSchedule\"")
	require.NoError(t, err)

	// check variables overrides
	cmd = strings.Split("zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'", " ")
	outputUIColor, _, err := e2e.UDS(cmd...)
	require.Equal(t, "'green, yellow'", outputUIColor)
	require.NoError(t, err)

	// check variables overrides, no default but set in config
	cmd = strings.Split("zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_MESSAGE\")].value}'", " ")
	outputMsg, _, err := e2e.UDS(cmd...)
	require.Equal(t, "'Hello Unicorn'", outputMsg)
	require.NoError(t, err)

	// check variables overrides, no default and not set in config
	cmd = strings.Split("zarf tools kubectl get secret test-secret -n podinfo -o jsonpath=\"{.data.test}\"", " ")
	secretValue, _, err := e2e.UDS(cmd...)
	// expect the value to be from the underlying chart's values.yaml, no overrides
	require.Equal(t, "\"dGVzdC1zZWNyZXQ=\"", secretValue)
	require.NoError(t, err)

	// check variables overrides with an object-type value
	cmd = strings.Split("zarf tools kubectl get deployment -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].securityContext}'", " ")
	securityContext, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	require.Contains(t, securityContext, "NET_ADMIN")
	require.Contains(t, securityContext, "\"runAsGroup\":4000")

	// check variables overrides with a list-type value
	cmd = strings.Split("zarf tools kubectl get ingress -n podinfo unicorn-podinfo -o=jsonpath='{.spec.rules[*].host}''", " ")
	hosts, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	require.Contains(t, hosts, "podinfo.burning.boats")
	require.Contains(t, hosts, "podinfo.unicorns")

	remove(t, bundlePath)
}

func TestBundleWithEnvVarHelmOverrides(t *testing.T) {
	// set up configs and env vars
	deployZarfInit(t)
	e2e.HelmDepUpdate(t, "src/test/packages/helm/unicorn-podinfo")
	e2e.CreateZarfPkg(t, "src/test/packages/helm", false)
	color := "purple"
	b64Secret := "dGhhdCBhaW50IG15IHRydWNr"
	err := os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/07-helm-overrides", "uds-config.yaml"))
	err = os.Setenv("UDS_UI_COLOR", color)
	err = os.Setenv("UDS_SECRET_VAL", b64Secret)
	require.NoError(t, err)

	// create and deploy bundle
	bundleDir := "src/test/bundles/07-helm-overrides"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-helm-overrides-%s-0.0.1.tar.zst", e2e.Arch))
	createLocal(t, bundleDir, e2e.Arch)
	deploy(t, bundlePath)

	// check override variables, ensure they are coming from env vars and take highest precedence
	cmd := strings.Split("zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'", " ")
	outputUIColor, _, err := e2e.UDS(cmd...)
	require.Equal(t, fmt.Sprintf("'%s'", color), outputUIColor)
	require.NoError(t, err)

	cmd = strings.Split("zarf tools kubectl get secret test-secret -n podinfo -o jsonpath=\"{.data.test}\"", " ")
	secretValue, _, err := e2e.UDS(cmd...)
	require.Equal(t, fmt.Sprintf("\"%s\"", b64Secret), secretValue)
	require.NoError(t, err)

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

	// test env var taking highest precedence
	cmd := strings.Split("zarf tools kubectl get deploy -n podinfo unicorn-podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_UI_COLOR\")].value}'", " ")
	outputUIColor, _, err := e2e.UDS(cmd...)
	require.Equal(t, fmt.Sprintf("'%s'", color), outputUIColor)
	require.NoError(t, err)

	// test uds-config.variables overriding a shared var
	require.Contains(t, stderr, "shared var in output-var pkg: unicorns.uds.dev")

	// test uds-config.shared overriding a Zarf var
	require.Contains(t, stderr, "shared var in helm-overrides pkg: burning.boats")

	// test uds-config.shared overriding values in a Helm chart (ie. bundle overrides)
	cmd = strings.Split("zarf tools kubectl get deploy unicorn-podinfo -n podinfo -o=jsonpath='{.spec.template.spec.containers[0].env[?(@.name==\"PODINFO_BACKEND_URL\")].value}'", " ")
	backend, _, err := e2e.UDS(cmd...)
	require.Equal(t, fmt.Sprintf("'%s'", "burning.boats"), backend)
	require.NoError(t, err)

	remove(t, bundlePath)
}
