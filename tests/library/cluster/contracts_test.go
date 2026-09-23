// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library && cluster_integration && !owned_cluster

package cluster_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/state"
	"github.com/zarf-dev/zarf/src/types"
	"oras.land/oras-go/v2/registry"
)

func TestInClusterDeploySourceAndRemove(t *testing.T) {
	client := suppliedCluster(t)
	fixture := createClusterBundleFixture(t)
	createClusterNamespace(t, client, fixture.Namespace)
	registerClusterFixtureCleanup(t, client, fixture)
	source := prepareClusterDeploySource(t, fixture.Root, fixture.Config)
	events := &clusterHookEvents{}

	result, err := bundle.Deploy(t.Context(), source, bundle.DeployOptions{
		Config: fixture.Config,
		BundleDeployHooks: bundle.BundleDeployHooks{
			PreDeploy: func(context.Context, *spec.UDSBundle, *bundle.DeployOptions) error {
				events.add("bundle-pre")
				return nil
			},
			PostDeploy: func(context.Context, *spec.UDSBundle) error {
				events.add("bundle-post")
				return nil
			},
		},
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy: func(_ context.Context, pkg *spec.Package, _ *bundle.ZarfPackageLayout, opts *bundle.DeployPackageOptions) error {
				if opts.Config.Options.Concurrency != 1 || opts.Config.Variables["scalar"] != "configured" {
					return fmt.Errorf("unexpected hook configuration")
				}
				events.add("package-pre:" + pkg.Name)
				return nil
			},
			PostDeploy: func(_ context.Context, pkg *spec.Package) error {
				events.add("package-post:" + pkg.Name)
				return nil
			},
		},
	})
	require.NoError(t, err)
	assertClusterDeployment(t, client, fixture, result)
	events.assertDependencyOrder(t, fixture)

	removed, err := bundle.Remove(t.Context(), source, bundle.RemoveOptions{Config: fixture.Config, Force: true})
	require.NoError(t, err)
	assertRemoval(t, fixture, removed, bundle.RemovePackageStatusRemoved, bundle.RemovePackageStatusRemoved)
	assertClusterConfigMapsAbsent(t.Context(), t, client, fixture)
}

func TestInClusterDeployArtifactWithCustomLoaderAndRemove(t *testing.T) {
	client := suppliedCluster(t)
	fixture := createClusterBundleFixture(t)
	createClusterNamespace(t, client, fixture.Namespace)
	registerClusterFixtureCleanup(t, client, fixture)
	source := prepareClusterDeploySource(t, fixture.ArtifactPath, fixture.Config)
	require.NotNil(t, source.Loader)
	stagingRoot := t.TempDir()
	loader := &clusterContractLoader{
		delegate: source.Loader,
		root:     stagingRoot,
		digests:  resolveClusterPackageDigests(t, fixture, startClusterRegistry(t)),
		paths:    map[string]string{},
	}
	source.Loader = loader

	result, err := bundle.Deploy(t.Context(), source, bundle.DeployOptions{
		Config: fixture.Config,
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy: loader.setResolvedDigest,
		},
	})
	require.NoError(t, err)
	assertClusterDeployment(t, client, fixture, result)
	for _, packageName := range []string{fixture.BasePackage, fixture.AppPackage} {
		assert.NotEmpty(t, loader.digest(packageName))
		assert.True(t, filepath.IsAbs(loader.path(packageName)))
		assertStagedUnder(t, stagingRoot, loader.path(packageName))
		assert.Equal(t, loader.digest(packageName), deployedPackageDigest(t, client, packageName, fixture.Namespace))
	}

	removed, err := bundle.Remove(t.Context(), &bundle.DeploySource{BundlePath: fixture.ArtifactPath}, bundle.RemoveOptions{
		Config:                    fixture.Config,
		Force:                     true,
		SkipSignatureVerification: true,
	})
	require.NoError(t, err)
	assertRemoval(t, fixture, removed, bundle.RemovePackageStatusRemoved, bundle.RemovePackageStatusRemoved)
	assertClusterConfigMapsAbsent(t.Context(), t, client, fixture)
}

func TestInClusterPullPrepareDeployAndRemoveOCI(t *testing.T) {
	client := suppliedCluster(t)
	fixture := createClusterBundleFixture(t)
	createClusterNamespace(t, client, fixture.Namespace)
	registerClusterFixtureCleanup(t, client, fixture)
	config := clusterBundleConfig(t, true)
	ref := fmt.Sprintf("%s/uds-cli-302/%s:v1", startClusterRegistry(t), fixture.BundleName)
	pushed, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	require.NotNil(t, pushed)
	pulled, err := bundle.Pull(t.Context(), pushed.OCIReference, t.TempDir(), bundle.PullOptions{
		Config:                    config,
		SkipSignatureVerification: true,
	})
	require.NoError(t, err)
	require.FileExists(t, pulled.OutputPath)
	source := prepareClusterDeploySource(t, pulled.OutputPath, config)

	result, err := bundle.Deploy(t.Context(), source, bundle.DeployOptions{Config: config})
	require.NoError(t, err)
	assertClusterDeployment(t, client, fixture, result)

	removed, err := bundle.Remove(t.Context(), &bundle.DeploySource{BundlePath: "oci://" + pushed.OCIReference}, bundle.RemoveOptions{
		Config:                    config,
		Force:                     true,
		SkipSignatureVerification: true,
	})
	require.NoError(t, err)
	assertRemoval(t, fixture, removed, bundle.RemovePackageStatusRemoved, bundle.RemovePackageStatusRemoved)
	assertClusterConfigMapsAbsent(t.Context(), t, client, fixture)
}

func TestInClusterRemoveReportsSkippedAndMixedResults(t *testing.T) {
	client := suppliedCluster(t)
	fixture := createClusterBundleFixture(t)
	createClusterNamespace(t, client, fixture.Namespace)
	registerClusterFixtureCleanup(t, client, fixture)
	source := prepareClusterDeploySource(t, fixture.Root, fixture.Config)

	result, err := bundle.Deploy(t.Context(), source, bundle.DeployOptions{Config: fixture.Config})
	require.NoError(t, err)
	assertClusterDeployment(t, client, fixture, result)
	removedApp, err := bundle.Remove(t.Context(), source, bundle.RemoveOptions{
		Config:   fixture.Config,
		Packages: []string{fixture.AppPackage},
	})
	require.NoError(t, err)
	require.Len(t, removedApp.Packages, 1)
	assert.Equal(t, bundle.RemovePackageStatusRemoved, removedApp.Packages[0].Status)

	mixed, err := bundle.Remove(t.Context(), source, bundle.RemoveOptions{
		Config:   fixture.Config,
		Packages: []string{fixture.BasePackage, fixture.AppPackage},
		Force:    true,
	})
	require.NoError(t, err)
	assert.Equal(t, fixture.BundleName, mixed.BundleName)
	assert.ElementsMatch(t, []bundle.RemovePackageResult{
		{Name: fixture.BasePackage, Status: bundle.RemovePackageStatusRemoved},
		{Name: fixture.AppPackage, Status: bundle.RemovePackageStatusSkipped},
	}, mixed.Packages)
	for _, packageResult := range mixed.Packages {
		if packageResult.Name == fixture.BasePackage {
			assert.Equal(t, "removed", string(packageResult.Status))
		}
		if packageResult.Name == fixture.AppPackage {
			assert.Equal(t, "skipped", string(packageResult.Status))
		}
	}
	allAbsent, err := bundle.Remove(t.Context(), source, bundle.RemoveOptions{
		Config:   fixture.Config,
		Packages: []string{fixture.BasePackage, fixture.AppPackage},
		Force:    true,
	})
	require.NoError(t, err)
	assertRemoval(t, fixture, allAbsent, bundle.RemovePackageStatusSkipped, bundle.RemovePackageStatusSkipped)
	for _, packageResult := range allAbsent.Packages {
		assert.Equal(t, "skipped", string(packageResult.Status))
	}
	assertClusterConfigMapsAbsent(t.Context(), t, client, fixture)
}

type clusterContractLoader struct {
	delegate bundle.ZarfPackageLayoutLoader
	root     string

	mu      sync.Mutex
	digests map[string]string
	paths   map[string]string
}

var (
	_ bundle.ZarfPackageLayoutLoader    = (*clusterContractLoader)(nil)
	_ bundle.PackageStagingRootProvider = (*clusterContractLoader)(nil)
)

func (l *clusterContractLoader) PackageStagingRoot(context.Context) string { return l.root }

func (l *clusterContractLoader) LoadPackageLayout(ctx context.Context, pkg *spec.Package, dst string, opts bundle.ZarfPackageLayoutLoadOptions) (*bundle.ZarfPackageLayoutLoadResult, error) {
	result, err := l.delegate.LoadPackageLayout(ctx, pkg, dst, opts)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	digest := l.digests[pkg.Name]
	l.paths[pkg.Name] = dst
	l.mu.Unlock()
	if digest == "" {
		return nil, fmt.Errorf("no registry-resolved digest for package %q", pkg.Name)
	}
	result.Layout.SetDeployedDigest("")
	return result, nil
}

func (l *clusterContractLoader) setResolvedDigest(_ context.Context, pkg *spec.Package, layout *bundle.ZarfPackageLayout, _ *bundle.DeployPackageOptions) error {
	if layout.Digest() != "" {
		return fmt.Errorf("loaded package %q retained a deployed digest", pkg.Name)
	}
	digest := l.digest(pkg.Name)
	if digest == "" {
		return fmt.Errorf("loaded package %q did not retain its resolved digest", pkg.Name)
	}
	layout.SetDeployedDigest(digest)
	return nil
}

func (l *clusterContractLoader) digest(packageName string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.digests[packageName]
}

func (l *clusterContractLoader) path(packageName string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.paths[packageName]
}

func resolveClusterPackageDigests(t *testing.T, fixture clusterBundleFixture, registryHost string) map[string]string {
	t.Helper()
	destination := registry.Reference{Registry: registryHost, Repository: "uds-cli-302-packages"}
	remoteOptions := types.RemoteOptions{PlainHTTP: true}
	digests := make(map[string]string, 2)
	for packageName, source := range map[string]string{
		fixture.BasePackage: fixture.BaseSource,
		fixture.AppPackage:  fixture.AppSource,
	} {
		local, err := packager.LoadPackage(t.Context(), source, packager.LoadOptions{
			Architecture:  runtime.GOARCH,
			CachePath:     t.TempDir(),
			RemoteOptions: remoteOptions,
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, local.Cleanup()) })
		published, err := packager.PublishPackage(t.Context(), local, destination, packager.PublishPackageOptions{
			Retries:       1,
			RemoteOptions: remoteOptions,
		})
		require.NoError(t, err)
		remote, err := packager.LoadPackage(t.Context(), published.String(), packager.LoadOptions{
			Architecture:  runtime.GOARCH,
			CachePath:     t.TempDir(),
			RemoteOptions: remoteOptions,
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, remote.Cleanup()) })
		digests[packageName] = remote.Digest()
		require.NotEmpty(t, digests[packageName])
	}
	return digests
}

type clusterHookEvents struct {
	mu     sync.Mutex
	events []string
}

func (e *clusterHookEvents) add(event string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
}

func (e *clusterHookEvents) assertDependencyOrder(t *testing.T, fixture clusterBundleFixture) {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	require.Equal(t, []string{
		"bundle-pre",
		"package-pre:" + fixture.BasePackage,
		"package-post:" + fixture.BasePackage,
		"package-pre:" + fixture.AppPackage,
		"package-post:" + fixture.AppPackage,
		"bundle-post",
	}, e.events)
}

func createClusterNamespace(t *testing.T, client *kubernetes.Clientset, namespace string) {
	t.Helper()
	_, err := client.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		err := client.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			t.Errorf("delete test namespace %q: %v", namespace, err)
		}
	})
}

func registerClusterFixtureCleanup(t *testing.T, client *kubernetes.Clientset, fixture clusterBundleFixture) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		source, err := bundle.PrepareDeploySource(ctx, iostreams.IOStreams{}, fixture.Root, fixture.Config.Options.TmpDir, runtime.GOARCH)
		if err != nil {
			t.Errorf("prepare cleanup source: %v", err)
			return
		}
		defer func() {
			if closeErr := source.Close(); closeErr != nil {
				t.Errorf("close cleanup source: %v", closeErr)
			}
		}()
		_, err = bundle.Remove(ctx, source, bundle.RemoveOptions{Config: fixture.Config, Force: true})
		if err != nil {
			t.Errorf("remove cleanup fixture: %v", err)
		}
		assertClusterConfigMapsAbsent(ctx, t, client, fixture)
	})
}

func prepareClusterDeploySource(t *testing.T, path string, config *bundle.UDSBundleConfig) *bundle.DeploySource {
	t.Helper()
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, path, config.Options.TmpDir, runtime.GOARCH)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, source.Close()) })
	return source
}

func assertClusterDeployment(t *testing.T, client *kubernetes.Clientset, fixture clusterBundleFixture, result *bundle.DeployResult) {
	t.Helper()
	require.NotNil(t, result)
	assert.Equal(t, fixture.BundleName, result.BundleName)
	require.Len(t, result.Packages, 2)
	assert.ElementsMatch(t, []string{fixture.BasePackage, fixture.AppPackage}, []string{result.Packages[0].Name, result.Packages[1].Name})
	assertClusterConfigMap(t, client, fixture.Namespace, fixture.BaseConfigMap)
	assertClusterConfigMap(t, client, fixture.Namespace, fixture.AppConfigMap)
}

func assertClusterConfigMap(t *testing.T, client *kubernetes.Clientset, namespace, name string) {
	t.Helper()
	configMap, err := client.CoreV1().ConfigMaps(namespace).Get(t.Context(), name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"scalar": "configured",
		"nested": "nested-configured",
		"items":  "first,second",
	}, configMap.Data)
}

func assertRemoval(t *testing.T, fixture clusterBundleFixture, result *bundle.RemoveResult, baseStatus, appStatus bundle.RemovePackageStatus) {
	t.Helper()
	require.NotNil(t, result)
	assert.Equal(t, fixture.BundleName, result.BundleName)
	assert.ElementsMatch(t, []bundle.RemovePackageResult{
		{Name: fixture.AppPackage, Status: appStatus},
		{Name: fixture.BasePackage, Status: baseStatus},
	}, result.Packages)
}

func assertClusterConfigMapsAbsent(ctx context.Context, t *testing.T, client *kubernetes.Clientset, fixture clusterBundleFixture) {
	t.Helper()
	for _, name := range []string{fixture.BaseConfigMap, fixture.AppConfigMap} {
		_, err := client.CoreV1().ConfigMaps(fixture.Namespace).Get(ctx, name, metav1.GetOptions{})
		require.Error(t, err)
		assert.True(t, apierrors.IsNotFound(err))
	}
}

func assertStagedUnder(t *testing.T, root, path string) {
	t.Helper()
	relative, err := filepath.Rel(root, path)
	require.NoError(t, err)
	assert.NotEqual(t, "..", relative)
	assert.NotContains(t, relative, ".."+string(filepath.Separator))
}

func deployedPackageDigest(t *testing.T, client *kubernetes.Clientset, packageName, namespace string) string {
	t.Helper()
	secretName := (&state.DeployedPackage{Name: packageName, NamespaceOverride: namespace}).GetSecretName()
	secret, err := client.CoreV1().Secrets(state.ZarfNamespaceName).Get(t.Context(), secretName, metav1.GetOptions{})
	require.NoError(t, err)
	var deployed state.DeployedPackage
	require.NoError(t, json.Unmarshal(secret.Data["data"], &deployed))
	return deployed.Digest
}
