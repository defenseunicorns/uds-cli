// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
)

const variableDeployTimeout = 5 * time.Minute

func TestBundleWithHelmOverrides(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/07-helm-overrides", namespace)
	packageDir := filepath.Clean(filepath.Join(bundleDir, "../../packages/helm"))
	scopeHelmPackage(t, bundleDir, packageDir, namespace)
	prepareHelmDependencies(t, opts, packageDir)
	opts.Env["UDS_CONFIG"] = filepath.Join(bundleDir, "uds-config.yaml")
	bundlePath := createClusterBundle(t, opts, bundleDir, "../../packages/helm")
	defer removeBundle(t, opts, bundlePath, nil)

	result := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--retries", "1")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "unicorn-podinfo", variableDeployTimeout)

	deployment := k8s.Deployment(namespace, "unicorn-podinfo")
	require.Equal(t, int32(2), *deployment.Spec.Replicas)
	require.Equal(t, "customValue", deployment.Spec.Template.Annotations["customAnnotation"])
	require.Contains(t, deployment.Spec.Template.Spec.Containers[0].Args, "--level=debug")
	require.Equal(t, "green, yellow", containerEnv(deployment.Spec.Template.Spec.Containers[0].Env, "PODINFO_UI_COLOR"))
	require.Equal(t, "Hello Unicorn", containerEnv(deployment.Spec.Template.Spec.Containers[0].Env, "PODINFO_UI_MESSAGE"))
	require.Equal(t, int64(4000), *deployment.Spec.Template.Spec.Containers[0].SecurityContext.RunAsGroup)
	require.Contains(t, deployment.Spec.Template.Spec.Containers[0].SecurityContext.Capabilities.Add, corev1.Capability("NET_ADMIN"))
	require.Equal(t, []byte("test-secret"), k8s.Secret(namespace, "test-secret").Data["test"])
	require.Contains(t, string(k8s.Secret(namespace, "test-file-secret").Data["test"]), "ssh-rsa")
	require.Contains(t, string(k8s.Secret(namespace, "second-chart-secret").Data["test"]), "ssh-rsa")

	hosts := k8s.Ingress(namespace, "unicorn-podinfo").Spec.Rules
	require.Len(t, hosts, 2)
	require.Equal(t, "podinfo.burning.boats", hosts[0].Host)
	require.Equal(t, "podinfo.unicorns", hosts[1].Host)
}

func TestBundleWithHelmOverridesValuesFile(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/07-helm-overrides/values-file", namespace)
	packageDir := filepath.Clean(filepath.Join(bundleDir, "../../../packages/helm"))
	scopeHelmPackage(t, bundleDir, packageDir, namespace)
	prepareHelmDependencies(t, opts, packageDir)
	opts.Env["UDS_CONFIG"] = testutil.CopyFixture(t, "bundles/07-helm-overrides/uds-config.yaml")
	bundlePath := createClusterBundle(t, opts, bundleDir, "../../../packages/helm")
	defer removeBundle(t, opts, bundlePath, nil)

	result := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--retries", "1")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "unicorn-podinfo", variableDeployTimeout)
	deployment := k8s.Deployment(namespace, "unicorn-podinfo")
	require.Equal(t, int32(2), *deployment.Spec.Replicas)
	require.Equal(t, "customValue2", deployment.Spec.Template.Annotations["customAnnotation"])
	require.Len(t, deployment.Spec.Template.Spec.Tolerations, 2)
	require.Contains(t, deployment.Spec.Template.Spec.Tolerations, corev1.Toleration{
		Key: "uds", Operator: corev1.TolerationOpEqual, Value: "true", Effect: corev1.TaintEffectNoSchedule,
	})
	require.Contains(t, deployment.Spec.Template.Spec.Tolerations, corev1.Toleration{
		Key: "unicorn", Operator: corev1.TolerationOpEqual, Value: "defense", Effect: corev1.TaintEffectNoSchedule,
	})
}

func TestBundleWithPackageNamespaceOverride(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/07-helm-overrides/package-namespace", namespace)
	bundlePath := createClusterBundle(t, opts, bundleDir, "../../../packages/nginx-namespace-override")
	defer removeBundle(t, opts, bundlePath, nil)

	result := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "nginx-deployment", variableDeployTimeout)
	removed := runClusterCLI(t, opts, "remove", bundlePath, "--confirm", "--insecure")
	require.NoError(t, removed.Err, removed.Stderr)
	k8s.AssertDeploymentDoesNotExist(namespace, "nginx-deployment")
}

func TestBundleWithDuplicatePackages(t *testing.T) {
	t.Parallel()

	namespaceA, k8s := testutil.AllocateNamespace(t, suite)
	namespaceB, _ := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	registry := testutil.NewRegistry(t)
	repository := registry.Host + "/variables/nginx-namespace-override"
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/07-helm-overrides/duplicate", namespaceA)
	setBundlePackageNamespaces(t, bundleDir, []string{namespaceA, namespaceB})
	rewriteBundleNamespaces(t, bundleDir, map[string]string{
		"override-ns":         namespaceA,
		"another-override-ns": namespaceB,
	})
	setPackageRepository(t, bundleDir, "helm-overrides", repository)
	setPackageRepository(t, bundleDir, "helm-overrides-duplicate", repository)
	clearPackageOverrides(t, bundleDir)

	packageDir := testutil.CopyFixture(t, "packages/nginx-namespace-override")
	archive := createClusterPackage(t, opts, packageDir, t.TempDir())
	publishPackage(t, opts, archive, registry.Host+"/variables")
	bundlePath := createClusterBundle(t, opts, bundleDir)
	defer removeBundle(t, opts, bundlePath, nil)

	assertDeployments := func() {
		k8s.WaitForDeploymentReady(namespaceA, "nginx-deployment", variableDeployTimeout)
		k8s.WaitForDeploymentReady(namespaceB, "nginx-deployment", variableDeployTimeout)
	}
	local := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--insecure")
	require.NoError(t, local.Err, local.Stderr)
	assertDeployments()
	localRemove := runClusterCLI(t, opts, "remove", bundlePath, "--confirm", "--insecure")
	require.NoError(t, localRemove.Err, localRemove.Stderr)
	k8s.AssertDeploymentDoesNotExist(namespaceA, "nginx-deployment")
	k8s.AssertDeploymentDoesNotExist(namespaceB, "nginx-deployment")

	bundleRepository := registry.Host + "/variables/duplicates"
	published := runClusterCLI(t, opts, "publish", bundlePath, bundleRepository, "--insecure")
	require.NoError(t, published.Err, published.Stderr)
	// UDS publish appends the bundle name to the destination repository.
	remoteRef := bundleRepository + "/duplicates:0.0.1"
	remote := runClusterCLI(t, opts, "deploy", remoteRef, "--confirm", "--insecure")
	require.NoError(t, remote.Err, remote.Stderr)
	assertDeployments()
	remoteRemove := runClusterCLI(t, opts, "remove", remoteRef, "--confirm", "--insecure")
	require.NoError(t, remoteRemove.Err, remoteRemove.Stderr)
}

func TestBundleWithEnvVarHelmOverrides(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/07-helm-overrides", namespace)
	packageDir := filepath.Clean(filepath.Join(bundleDir, "../../packages/helm"))
	scopeHelmPackage(t, bundleDir, packageDir, namespace)
	prepareHelmDependencies(t, opts, packageDir)
	opts.Env["UDS_CONFIG"] = filepath.Join(bundleDir, "uds-config.yaml")
	opts.Env["UDS_UI_COLOR"] = "purple"
	opts.Env["UDS_UI_MSG"] = "set by a child environment"
	opts.Env["UDS_SECRET_VAL"] = "dGhhdCBhaW50IG15IHRydWNrCg=="
	bundlePath := createClusterBundle(t, opts, bundleDir, "../../packages/helm")
	defer removeBundle(t, opts, bundlePath, nil)

	result := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "unicorn-podinfo", variableDeployTimeout)
	require.Equal(t, "purple", containerEnv(k8s.Deployment(namespace, "unicorn-podinfo").Spec.Template.Spec.Containers[0].Env, "PODINFO_UI_COLOR"))
	require.Equal(t, []byte("that aint my truck\n"), k8s.Secret(namespace, "test-secret").Data["test"])

	overridden := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--set", "UI_COLOR=orange", "--set", "helm-overrides.UI_MSG=foo")
	require.NoError(t, overridden.Err, overridden.Stderr)
	deployment := k8s.Deployment(namespace, "unicorn-podinfo")
	require.Equal(t, "orange", containerEnv(deployment.Spec.Template.Spec.Containers[0].Env, "PODINFO_UI_COLOR"))
	require.Equal(t, "foo", containerEnv(deployment.Spec.Template.Spec.Containers[0].Env, "PODINFO_UI_MESSAGE"))
}

func TestVariablePrecedence(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/08-var-precedence", namespace)
	packageDir := filepath.Clean(filepath.Join(bundleDir, "../../packages/helm"))
	scopeHelmPackage(t, bundleDir, packageDir, namespace)
	prepareHelmDependencies(t, opts, packageDir)
	opts.Env["UDS_CONFIG"] = filepath.Join(bundleDir, "uds-config.yaml")
	opts.Env["UDS_UI_COLOR"] = "green"
	bundlePath := createClusterBundle(t, opts, bundleDir, "../../packages/helm", "../../packages/no-cluster/output-var")
	defer removeBundle(t, opts, bundlePath, nil)

	result := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--retries", "1")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "unicorn-podinfo", variableDeployTimeout)
	require.Contains(t, result.Stderr, "shared var in output-var pkg: unicorns.uds.dev")
	require.Contains(t, result.Stderr, "shared var in helm-overrides pkg: burning.boats")
	container := k8s.Deployment(namespace, "unicorn-podinfo").Spec.Template.Spec.Containers[0]
	require.Equal(t, "green", containerEnv(container.Env, "PODINFO_UI_COLOR"))
	require.Equal(t, "burning.boats", containerEnv(container.Env, "PODINFO_BACKEND_URL"))
}

func TestExportVarsAsGlobalVars(t *testing.T) {
	t.Parallel()

	namespace, k8s := testutil.AllocateNamespace(t, suite)
	opts := isolatedClusterOptions(t)
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/12-exported-pkg-vars", namespace)
	packageDir := filepath.Clean(filepath.Join(bundleDir, "../../packages/helm"))
	scopeHelmPackage(t, bundleDir, packageDir, namespace)
	prepareHelmDependencies(t, opts, packageDir)
	bundlePath := createClusterBundle(t, opts, bundleDir, "../../packages/no-cluster/output-var", "../../packages/helm")
	defer removeBundle(t, opts, bundlePath, nil)

	result := runClusterCLI(t, opts, "deploy", bundlePath, "--confirm", "--retries", "1")
	require.NoError(t, result.Err, result.Stderr)
	k8s.WaitForDeploymentReady(namespace, "unicorn-podinfo", variableDeployTimeout)
	deployment := k8s.Deployment(namespace, "unicorn-podinfo")
	require.Equal(t, "orange", containerEnv(deployment.Spec.Template.Spec.Containers[0].Env, "PODINFO_UI_COLOR"))
	require.Equal(t, "orangeAnnotation", deployment.Spec.Template.Annotations["customAnnotation"])
	require.Len(t, deployment.Spec.Template.Spec.Tolerations, 2)
}

func TestScopeHelmPackageKeepsPackageNamespaceOverride(t *testing.T) {
	namespace := "uds-it-helm-override"
	bundleDir := testutil.PrepareBundleForNamespace(t, "bundles/07-helm-overrides", namespace)
	packageDir := filepath.Clean(filepath.Join(bundleDir, "../../packages/helm"))

	scopeHelmPackage(t, bundleDir, packageDir, namespace)

	var bundle types.UDSBundle
	require.NoError(t, utils.ReadYAMLStrict(filepath.Join(bundleDir, config.BundleYAML), &bundle))
	for _, pkg := range bundle.Packages {
		if pkg.Name == "helm-overrides" {
			require.Equal(t, namespace, pkg.Namespace)
			return
		}
	}
	require.Fail(t, "helm-overrides package not found")
}

func prepareHelmDependencies(t *testing.T, opts testutil.CommandOptions, packageDir string) {
	t.Helper()
	chartDir := filepath.Join(packageDir, "unicorn-podinfo")
	added := runClusterCLI(t, opts, "zarf", "tools", "helm", "repo", "add", "podinfo", "https://stefanprodan.github.io/podinfo")
	require.NoError(t, added.Err, added.Stderr)
	updated := runClusterCLI(t, opts, "zarf", "tools", "helm", "dependency", "update", chartDir)
	require.NoError(t, updated.Err, updated.Stderr)
}

// scopeHelmPackage keeps the two charts in this fixture in the test namespace.
// A UDS package-level namespace override cannot apply to a package that declares
// more than one chart namespace.
func scopeHelmPackage(t *testing.T, bundleDir, packageDir, namespace string) {
	t.Helper()
	zarfPath := filepath.Join(packageDir, "zarf.yaml")
	contents, err := os.ReadFile(zarfPath)
	require.NoError(t, err)
	contents = []byte(strings.ReplaceAll(string(contents), "namespace: podinfo", "namespace: "+namespace))
	contents = []byte(strings.ReplaceAll(string(contents), "namespace: second-chart", "namespace: "+namespace))
	require.NoError(t, os.WriteFile(zarfPath, contents, 0o600))

	bundlePath := filepath.Join(bundleDir, config.BundleYAML)
	var bundle types.UDSBundle
	require.NoError(t, utils.ReadYAMLStrict(bundlePath, &bundle))
	for index := range bundle.Packages {
		if bundle.Packages[index].Name == "helm-overrides" {
			bundle.Packages[index].Namespace = namespace
		}
	}
	require.NoError(t, zarfUtils.WriteYaml(bundlePath, &bundle, 0o600))
}

func clearPackageOverrides(t *testing.T, bundleDir string) {
	t.Helper()
	bundlePath := filepath.Join(bundleDir, config.BundleYAML)
	var bundle types.UDSBundle
	require.NoError(t, utils.ReadYAMLStrict(bundlePath, &bundle))
	for index := range bundle.Packages {
		bundle.Packages[index].Overrides = nil
	}
	require.NoError(t, zarfUtils.WriteYaml(bundlePath, &bundle, 0o600))
}

func rewriteBundleNamespaces(t *testing.T, bundleDir string, replacements map[string]string) {
	t.Helper()
	path := filepath.Join(bundleDir, "uds-bundle.yaml")
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	updated := string(contents)
	for oldNamespace, newNamespace := range replacements {
		updated = strings.ReplaceAll(updated, oldNamespace, newNamespace)
	}
	require.NoError(t, os.WriteFile(path, []byte(updated), 0o600))
}

func containerEnv(environment []corev1.EnvVar, name string) string {
	for _, variable := range environment {
		if variable.Name == name {
			return variable.Value
		}
	}
	return ""
}
