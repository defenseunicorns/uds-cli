// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package integration

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

func TestCreateAndPublishToLocalRegistry(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	registry := testutil.NewRegistry(t)
	repository := uniqueRepository(t)

	for _, architecture := range []string{"amd64", "arm64"} {
		bundleDir := localBundleFixture(t, workspace, architecture)
		bundlePath := createBundle(t, workspace, bundleDir, repository, architecture)
		publish := runBundleCLI(t, workspace, "publish", bundlePath, registry.Host, "--insecure", "--no-progress")
		require.NoError(t, publish.Err, publish.Stderr)
	}

	index := registryIndex(t, registry.Host, repository)
	require.Equal(t, ocispec.MediaTypeImageIndex, index.MediaType)
	require.Len(t, index.Manifests, 2)

	architectures := map[string]bool{}
	for _, manifest := range index.Manifests {
		require.Equal(t, ocispec.MediaTypeImageManifest, manifest.MediaType)
		require.NotNil(t, manifest.Platform)
		architectures[manifest.Platform.Architecture] = true
	}
	require.Equal(t, map[string]bool{"amd64": true, "arm64": true}, architectures)

	remoteRef := "oci://" + registry.Host + "/" + repository + ":0.0.1"
	inspect := runBundleCLI(t, workspace, "inspect", remoteRef, "--insecure", "--no-color", "--no-progress")
	require.NoError(t, inspect.Err, inspect.Stderr)
	require.Contains(t, inspect.Stdout, repository)
	inspectSBOM := runBundleCLI(t, workspace, "inspect", remoteRef, "--sbom", "--insecure", "--no-color", "--no-progress")
	require.NoError(t, inspectSBOM.Err, inspectSBOM.Stderr)
	require.Contains(t, inspectSBOM.Stdout+inspectSBOM.Stderr, "No SBOMs found in bundle")

	pullDir := filepath.Join(workspace, "pulled")
	require.NoError(t, os.MkdirAll(pullDir, 0o700))
	pull := runBundleCLI(t, workspace, "pull", registry.Host+"/"+repository+":0.0.1", "--insecure", "--output", pullDir, "--no-progress")
	require.NoError(t, pull.Err, pull.Stderr)
	require.FileExists(t, filepath.Join(pullDir, "uds-bundle-"+repository+"-"+runtime.GOARCH+"-0.0.1.tar.zst"))
}

func TestCreateWritesMultiArchitectureIndexToLocalRegistry(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	registry := testutil.NewRegistry(t)
	repository := uniqueRepository(t)

	for _, architecture := range []string{"amd64", "arm64"} {
		packageDir := testutil.CopyFixture(t, "packages/no-cluster/real-simple")
		packageResult := runZarfCLI(t, workspace,
			"zarf", "package", "create", packageDir,
			"--confirm", "--architecture", architecture,
			"--output", packageDir,
			"--cache", filepath.Join(workspace, "zarf-cache"),
			"--tmpdir", filepath.Join(workspace, "tmp"),
		)
		require.NoError(t, packageResult.Err, packageResult.Stderr)
		archive := filepath.Join(packageDir, "zarf-package-real-simple-"+architecture+"-0.0.1.tar.zst")

		published := runZarfCLI(t, workspace,
			"zarf", "package", "publish", archive, "oci://"+registry.Host+"/packages",
			"--plain-http", "--tmpdir", filepath.Join(workspace, "tmp"), "--cache", filepath.Join(workspace, "zarf-cache"),
		)
		require.NoError(t, published.Err, published.Stderr)

		bundleDir := filepath.Join(t.TempDir(), "bundle")
		require.NoError(t, os.MkdirAll(bundleDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "uds-bundle.yaml"), []byte(`kind: UDSBundle
metadata:
  name: `+repository+`
  version: 0.0.1
packages:
  - name: real-simple
    repository: `+registry.Host+`/packages/real-simple
    ref: 0.0.1
`), 0o600))

		result := runBundleCLI(t, workspace,
			"create", bundleDir,
			"--confirm", "--insecure", "--no-progress",
			"--architecture", architecture,
			"--name", repository,
			"--output", "oci://"+registry.Host,
		)
		require.NoError(t, result.Err, result.Stderr)
	}

	index := registryIndex(t, registry.Host, repository)
	require.Equal(t, ocispec.MediaTypeImageIndex, index.MediaType)
	require.Len(t, index.Manifests, 2)
}

func TestCreateUsesBundleInWorkingDirectory(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	registry := testutil.NewRegistry(t)
	packageDir := testutil.CopyFixture(t, "packages/no-cluster/real-simple")
	packageResult := runZarfCLI(t, workspace,
		"zarf", "package", "create", packageDir,
		"--confirm", "--architecture", runtime.GOARCH,
		"--output", packageDir,
		"--cache", filepath.Join(workspace, "zarf-cache"),
		"--tmpdir", filepath.Join(workspace, "tmp"),
	)
	require.NoError(t, packageResult.Err, packageResult.Stderr)

	archive := filepath.Join(packageDir, "zarf-package-real-simple-"+runtime.GOARCH+"-0.0.1.tar.zst")
	published := runZarfCLI(t, workspace,
		"zarf", "package", "publish", archive, "oci://"+registry.Host+"/packages",
		"--plain-http", "--tmpdir", filepath.Join(workspace, "tmp"), "--cache", filepath.Join(workspace, "zarf-cache"),
	)
	require.NoError(t, published.Err, published.Stderr)

	name := uniqueRepository(t)
	bundle := []byte(`kind: UDSBundle
metadata:
  name: ` + name + `
  version: 0.0.1
packages:
  - name: real-simple
    repository: ` + registry.Host + `/packages/real-simple
    ref: 0.0.1
`)
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "uds-bundle.yaml"), bundle, 0o600))

	result := runBundleCLI(t, workspace,
		"create", "--confirm", "--insecure", "--no-progress",
		"--architecture", runtime.GOARCH, "--output", filepath.Join(workspace, "bundles"),
	)
	require.NoError(t, result.Err, result.Stderr)
	require.FileExists(t, filepath.Join(workspace, "bundles", "uds-bundle-"+name+"-"+runtime.GOARCH+"-0.0.1.tar.zst"))
}

func TestBundlePreservesImageReferencesWhenPulled(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	registry := testutil.NewRegistry(t)
	packageDir := testutil.CopyFixture(t, "packages/podinfo-refs")
	packageResult := runZarfCLI(t, workspace,
		"zarf", "package", "create", packageDir,
		"--confirm", "--architecture", runtime.GOARCH,
		"--output", packageDir,
		"--cache", filepath.Join(workspace, "zarf-cache"),
		"--tmpdir", filepath.Join(workspace, "tmp"),
	)
	require.NoError(t, packageResult.Err, packageResult.Stderr)
	packagePath := filepath.Join(packageDir, "zarf-package-podinfo-ref-"+runtime.GOARCH+"-0.0.1.tar.zst")
	require.FileExists(t, packagePath)

	name := uniqueRepository(t)
	bundleDir := filepath.Join(workspace, "bundle")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	bundle := []byte("kind: UDSBundle\nmetadata:\n  name: " + name + "\n  version: 0.1.0\npackages:\n  - name: podinfo-ref\n    path: " + filepath.ToSlash(packagePath) + "\n    ref: 0.0.1\n")
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "uds-bundle.yaml"), bundle, 0o600))

	created := runBundleCLI(t, workspace, "create", bundleDir, "--confirm", "--insecure", "--architecture", runtime.GOARCH, "--output", bundleDir)
	require.NoError(t, created.Err, created.Stderr)
	bundlePath := filepath.Join(bundleDir, "uds-bundle-"+name+"-"+runtime.GOARCH+"-0.1.0.tar.zst")
	require.FileExists(t, bundlePath)

	published := runBundleCLI(t, workspace, "publish", bundlePath, registry.Host, "--insecure", "--no-progress")
	require.NoError(t, published.Err, published.Stderr)
	pullDir := filepath.Join(workspace, "pulled")
	pulled := runBundleCLI(t, workspace, "pull", registry.Host+"/"+name+":0.1.0", "--insecure", "--output", pullDir, "--no-progress")
	require.NoError(t, pulled.Err, pulled.Stderr)

	original, err := os.Stat(bundlePath)
	require.NoError(t, err)
	retrieved, err := os.Stat(filepath.Join(pullDir, filepath.Base(bundlePath)))
	require.NoError(t, err)
	require.LessOrEqual(t, int64(math.Abs(float64(original.Size()-retrieved.Size()))), int64(1024*1000), "the pulled bundle should retain its image content")
}

func registryIndex(t *testing.T, host, repository string) ocispec.Index {
	t.Helper()

	request, err := http.NewRequest(http.MethodGet, "http://"+host+"/v2/"+repository+"/manifests/0.0.1", nil)
	require.NoError(t, err)
	request.Header.Set("Accept", ocispec.MediaTypeImageIndex)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Equal(t, http.StatusOK, response.StatusCode)

	var index ocispec.Index
	require.NoError(t, json.NewDecoder(response.Body).Decode(&index))
	return index
}
