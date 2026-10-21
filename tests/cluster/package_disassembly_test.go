// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster_test

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/internal/zarf/modify"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
	"github.com/zarf-dev/zarf/src/pkg/state"
	"github.com/zarf-dev/zarf/src/pkg/transform"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	disassemblyLifecyclePackageName = "disassembly-lifecycle"
	disassemblyLifecycleImage       = "registry.k8s.io/e2e-test-images/busybox:1.29-4"
)

func TestPackageDisassemblyLifecycle(t *testing.T) {
	namespace, k8s := testutil.ReserveTestNamespace(t, namespaceCleanupTimeout)
	packageDir := t.TempDir()
	sourceDir, fileTarget := prepareLifecyclePackageSource(t)
	originalPath, err := packager.Create(t.Context(), sourceDir, packageDir, packager.CreateOptions{
		CachePath: t.TempDir(),
		SkipSBOM:  true,
	})
	require.NoError(t, err)

	worktree := filepath.Join(t.TempDir(), "disassembled")
	tmpDir := t.TempDir()
	_, err = modify.Disassemble(t.Context(), modify.Options{
		PackageOptions: modify.PackageOptions{TmpDir: tmpDir, Concurrency: 1},
		Source:         originalPath,
		OutputDir:      worktree,
	})
	require.NoError(t, err)
	definition, err := load.PackageDefinition(t.Context(), worktree, load.DefinitionOptions{})
	require.NoError(t, err)
	require.Len(t, definition.Components, 1)
	component := definition.Components[0]
	require.Len(t, component.Manifests, 1)
	require.Len(t, component.Manifests[0].Files, 2)
	require.Len(t, component.Charts, 1)
	require.NotNil(t, component.Charts[0].Local)
	require.Len(t, component.Files, 1)
	require.Len(t, component.Repositories, 1)
	replaceLifecycleValue(t, filepath.Join(worktree, filepath.FromSlash(component.Manifests[0].Files[0])), "revision: original", "revision: rebuilt")
	replaceLifecycleValue(t, filepath.Join(worktree, filepath.FromSlash(component.Manifests[0].Files[1])), "revision: original", "revision: rebuilt")
	replaceLifecycleValue(t, filepath.Join(worktree, filepath.FromSlash(component.Charts[0].Local.Path), "templates", "configmap.yaml"), "revision: original", "revision: rebuilt")
	replaceLifecycleValue(t, filepath.Join(worktree, filepath.FromSlash(component.Files[0].Source)), "original", "rebuilt")
	setLifecycleRepositoryRevision(t, component.Repositories[0].URL, "rebuilt")

	reassembled, err := modify.Reassemble(t.Context(), modify.ReassembleOptions{
		PackageOptions: modify.PackageOptions{TmpDir: tmpDir, Concurrency: 1},
		SourceDir:      worktree,
		Output:         packageDir,
	})
	require.NoError(t, err)

	var deployed api.Package
	removed := false
	t.Cleanup(func() {
		if removed || deployed.Metadata.Name == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), namespaceCleanupTimeout)
		defer cancel()
		client, err := cluster.New(ctx)
		if err != nil {
			t.Errorf("connect for disassembly package cleanup: %v", err)
			return
		}
		if err := packager.Remove(ctx, deployed, packager.RemoveOptions{Cluster: client, NamespaceOverride: namespace}); err != nil {
			t.Errorf("remove disassembly lifecycle package: %v", err)
		}
	})

	var seenPodUIDs []types.UID
	deployLifecyclePackage(t, originalPath, namespace, "original", &deployed)
	seenPodUIDs = assertLifecycleState(t, k8s, namespace, fileTarget, "original", seenPodUIDs)
	deployLifecyclePackage(t, reassembled, namespace, "rebuilt", &deployed)
	seenPodUIDs = assertLifecycleState(t, k8s, namespace, fileTarget, "rebuilt", seenPodUIDs)
	deployLifecyclePackage(t, originalPath, namespace, "original", &deployed)
	assertLifecycleState(t, k8s, namespace, fileTarget, "original", seenPodUIDs)

	client, err := cluster.New(t.Context())
	require.NoError(t, err)
	require.NoError(t, packager.Remove(t.Context(), deployed, packager.RemoveOptions{Cluster: client, NamespaceOverride: namespace}))
	assertLifecycleResourcesRemoved(t, k8s, namespace)
	_, err = client.GetDeployedPackage(t.Context(), disassemblyLifecyclePackageName, state.WithPackageNamespaceOverride(namespace))
	require.True(t, apierrors.IsNotFound(err), "expected deployed package state to be removed, got %v", err)
	removed = true
}

func prepareLifecyclePackageSource(t *testing.T) (string, string) {
	t.Helper()
	sourceDir := filepath.Join(t.TempDir(), "source")
	require.NoError(t, os.CopyFS(sourceDir, os.DirFS(testutil.TestDataPath("packages/disassemble-lifecycle"))))

	repositoryDir := filepath.Join(sourceDir, "repository")
	repository, err := git.PlainInit(repositoryDir, false)
	require.NoError(t, err)
	worktree, err := repository.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add("README.md")
	require.NoError(t, err)
	_, err = worktree.Commit("initial", &git.CommitOptions{Author: lifecycleCommitAuthor(1)})
	require.NoError(t, err)

	fileTarget := filepath.Join(t.TempDir(), "payload.txt")
	templatePath := filepath.Join(sourceDir, "zarf.yaml.tmpl")
	definition, err := os.ReadFile(templatePath)
	require.NoError(t, err)
	repositoryURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(repositoryDir)}).String()
	definition = []byte(strings.NewReplacer(
		"__FILE_TARGET__", strconv.Quote(fileTarget),
		"__REPOSITORY_URL__", strconv.Quote(repositoryURL),
	).Replace(string(definition)))
	//nolint:gosec // G703: sourceDir is a test-owned temporary copy of the fixed fixture.
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, layout.ZarfYAML), definition, 0o600))
	require.NoError(t, os.Remove(templatePath))
	return sourceDir, fileTarget
}

func deployLifecyclePackage(t *testing.T, path, namespace, repositoryRevision string, deployed *api.Package) {
	t.Helper()
	pkgLayout, err := packager.LoadPackage(t.Context(), path, packager.LoadOptions{})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()
	assertLifecycleRepository(t, pkgLayout, repositoryRevision)
	require.Equal(t, []string{disassemblyLifecycleImage}, pkgLayout.Definition().Components[0].GetImages())
	// The shared cluster has no optional Git server. Repository bytes are verified
	// from each package layout before the remaining primitives exercise deployment.
	pkgLayout.RemoveRepositories()
	*deployed = pkgLayout.Definition()
	_, err = packager.Deploy(t.Context(), pkgLayout, packager.DeployOptions{
		NamespaceOverride: namespace,
		Timeout:           5 * time.Minute,
	})
	require.NoError(t, err)
}

func assertLifecycleState(t *testing.T, k8s *testutil.K8sClient, namespace, fileTarget, want string, seenPodUIDs []types.UID) []types.UID {
	t.Helper()
	for _, name := range []string{disassemblyLifecyclePackageName + "-manifest", disassemblyLifecyclePackageName + "-chart"} {
		configMap, err := k8s.CoreV1().ConfigMaps(namespace).Get(t.Context(), name, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, want, configMap.Data["revision"])
	}
	payload, err := os.ReadFile(fileTarget)
	require.NoError(t, err)
	assert.Equal(t, want, strings.TrimSpace(string(payload)))
	k8s.WaitForDeploymentReady(namespace, disassemblyLifecyclePackageName, 2*time.Minute)
	deployment, err := k8s.AppsV1().Deployments(namespace).Get(t.Context(), disassemblyLifecyclePackageName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, want, deployment.Spec.Template.Labels["revision"])
	selector := "app=" + disassemblyLifecyclePackageName + ",revision=" + want
	pod := k8s.WaitForReadyPodBySelector(namespace, selector, 2*time.Minute, seenPodUIDs...)
	return append(seenPodUIDs, pod.UID)
}

func assertLifecycleRepository(t *testing.T, pkgLayout *layout.PackageLayout, want string) {
	t.Helper()
	definition := pkgLayout.Definition()
	require.Len(t, definition.Components, 1)
	require.Len(t, definition.Components[0].Repositories, 1)
	repository := definition.Components[0].Repositories[0]
	address := repository.LegacyURL
	if address == "" {
		address = repository.URL
	}
	name, err := transform.GitURLtoFolderName(address)
	require.NoError(t, err)
	repositoryRoot, err := pkgLayout.GetComponentDir(t.Context(), t.TempDir(), definition.Components[0].Name, layout.RepoComponentDir)
	require.NoError(t, err)
	contents, err := os.ReadFile(filepath.Join(repositoryRoot, name, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, want, strings.TrimSpace(string(contents)))
}

func setLifecycleRepositoryRevision(t *testing.T, rawURL, revision string) {
	t.Helper()
	repositoryURL, err := url.Parse(rawURL)
	require.NoError(t, err)
	require.Equal(t, "file", repositoryURL.Scheme)
	repositoryPath := filepath.FromSlash(repositoryURL.Path)
	repository, err := git.PlainOpen(repositoryPath)
	require.NoError(t, err)
	worktree, err := repository.Worktree()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte(revision+"\n"), 0o600))
	_, err = worktree.Add("README.md")
	require.NoError(t, err)
	_, err = worktree.Commit("set revision to "+revision, &git.CommitOptions{Author: lifecycleCommitAuthor(2)})
	require.NoError(t, err)
}

func lifecycleCommitAuthor(second int64) *object.Signature {
	return &object.Signature{Name: "UDS Test", Email: "test@example.com", When: time.Unix(second, 0)}
}

func replaceLifecycleValue(t *testing.T, path, oldValue, newValue string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	updated := bytes.Replace(contents, []byte(oldValue), []byte(newValue), 1)
	require.NotEqual(t, contents, updated)
	//nolint:gosec // G703: path comes from the test-owned disassembly directory.
	require.NoError(t, os.WriteFile(path, updated, 0o600))
}

func assertLifecycleResourcesRemoved(t *testing.T, k8s *testutil.K8sClient, namespace string) {
	t.Helper()
	for _, name := range []string{disassemblyLifecyclePackageName + "-manifest", disassemblyLifecyclePackageName + "-chart"} {
		_, err := k8s.CoreV1().ConfigMaps(namespace).Get(t.Context(), name, metav1.GetOptions{})
		assert.True(t, apierrors.IsNotFound(err))
	}
	_, err := k8s.AppsV1().Deployments(namespace).Get(t.Context(), disassemblyLifecyclePackageName, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
}
