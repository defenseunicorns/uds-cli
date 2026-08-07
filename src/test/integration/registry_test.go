// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package integration

import (
	"encoding/json"
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

	pullDir := filepath.Join(workspace, "pulled")
	require.NoError(t, os.MkdirAll(pullDir, 0o700))
	pull := runBundleCLI(t, workspace, "pull", registry.Host+"/"+repository+":0.0.1", "--insecure", "--output", pullDir, "--no-progress")
	require.NoError(t, pull.Err, pull.Stderr)
	require.FileExists(t, filepath.Join(pullDir, "uds-bundle-"+repository+"-"+runtime.GOARCH+"-0.0.1.tar.zst"))
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
