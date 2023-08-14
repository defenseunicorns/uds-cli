// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/defenseunicorns/zarf/src/pkg/utils/exec"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/zarf/src/pkg/utils"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry"
)

func zarfPublish(t *testing.T, path string, reg string) {
	cmd := "zarf"
	args := strings.Split(fmt.Sprintf("package publish %s oci://%s --insecure --oci-concurrency=10", path, reg), " ")
	tmp := exec.PrintCfg()
	_, _, err := exec.CmdWithContext(context.TODO(), tmp, cmd, args...)
	require.NoError(t, err)
}

const zarfVersion = "v0.28.3"

func TestBundleVariables(t *testing.T) {
	e2e.DownloadZarfInitPkg(t, zarfVersion)
	zarfPkgPath1 := "src/test/packages/zarf/no-cluster/output-var"
	zarfPkgPath2 := "src/test/packages/zarf/no-cluster/receive-var"
	e2e.CreateZarfPkg(t, zarfPkgPath1)
	e2e.CreateZarfPkg(t, zarfPkgPath2)

	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	pkg := filepath.Join(zarfPkgPath1, fmt.Sprintf("zarf-package-output-var-%s-0.0.1.tar.zst", e2e.Arch))
	zarfPublish(t, pkg, "localhost:888")

	pkg = filepath.Join(zarfPkgPath2, fmt.Sprintf("zarf-package-receive-var-%s-0.0.1.tar.zst", e2e.Arch))
	zarfPublish(t, pkg, "localhost:888")

	bundleDir := "src/test/packages/02-simple-vars"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-simple-vars-%s-0.0.1.tar.zst", e2e.Arch))

	os.Setenv("UDS_CONFIG", filepath.Join("src/test/packages/02-simple-vars", "uds-config.yaml"))

	create(t, bundleDir)
	_, stderr := deploy(t, bundlePath)

	require.Contains(t, stderr, "Received the following message: Unicorns are the national animal of Wales")
}

func TestBundle(t *testing.T) {
	e2e.SetupWithCluster(t)

	e2e.DownloadZarfInitPkg(t, zarfVersion)
	e2e.CreateZarfPkg(t, "src/test/packages/zarf/podinfo")

	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	e2e.SetupDockerRegistry(t, 889)
	defer e2e.TeardownRegistry(t, 889)

	pkg := fmt.Sprintf("src/test/packages/zarf/zarf-init-%s-%s.tar.zst", e2e.Arch, zarfVersion)
	zarfPublish(t, pkg, "localhost:888")

	pkg = fmt.Sprintf("src/test/packages/zarf/podinfo/zarf-package-podinfo-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:889")

	bundleDir := "src/test/packages/01-uds-bundle"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-example-%s-0.0.1.tar.zst", e2e.Arch))

	create(t, bundleDir) // todo: allow creating from both the folder containing and direct reference to uds-bundle.yaml
	inspect(t, bundlePath)
	deploy(t, bundlePath)
	remove(t, bundlePath)
}

func TestRemoteBundle(t *testing.T) {
	e2e.SetupWithCluster(t)

	e2e.DownloadZarfInitPkg(t, zarfVersion)
	e2e.CreateZarfPkg(t, "src/test/packages/zarf/podinfo")

	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	e2e.SetupDockerRegistry(t, 889)
	defer e2e.TeardownRegistry(t, 889)

	pkg := fmt.Sprintf("src/test/packages/zarf/zarf-init-%s-%s.tar.zst", e2e.Arch, zarfVersion)
	zarfPublish(t, pkg, "localhost:888")

	pkg = fmt.Sprintf("src/test/packages/zarf/podinfo/zarf-package-podinfo-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:889")

	bundleRef := registry.Reference{
		Registry: "localhost:888",
		// this info is derived from the bundle's metadata
		Repository: "example",
		Reference:  fmt.Sprintf("0.0.1-%s", e2e.Arch),
	}

	tarballPath := filepath.Join("build", fmt.Sprintf("uds-bundle-example-%s-0.0.1.tar.zst", e2e.Arch))
	bundlePath := "src/test/packages/01-uds-bundle"

	createRemote(t, bundlePath, bundleRef.Registry)
	pull(t, bundleRef.String(), tarballPath)
	inspectRemote(t, bundleRef.String())
	deployAndRemoveRemote(t, bundleRef.String(), tarballPath)
}

func create(t *testing.T, bundlePath string) {
	cmd := strings.Split(fmt.Sprintf("bundle create %s --set INIT_VERSION=%s --confirm --insecure", bundlePath, zarfVersion), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func createRemote(t *testing.T, bundlePath string, reg string) {
	cmd := strings.Split(fmt.Sprintf("bundle create %s -o oci://%s --set INIT_VERSION=%s --confirm --insecure", bundlePath, reg, zarfVersion), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func inspectRemote(t *testing.T, ref string) {
	cmd := strings.Split(fmt.Sprintf("bundle inspect oci://%s --insecure", ref), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func inspect(t *testing.T, tarballPath string) {
	cmd := strings.Split(fmt.Sprintf("bundle inspect %s", tarballPath), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func deploy(t *testing.T, tarballPath string) (stdout string, stderr string) {
	cmd := strings.Split(fmt.Sprintf("bundle deploy %s --confirm -l=debug", tarballPath), " ")
	stdout, stderr, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	return stdout, stderr
}

func remove(t *testing.T, tarballPath string) {
	cmd := strings.Split(fmt.Sprintf("bundle remove %s --confirm --insecure", tarballPath), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func deployAndRemoveRemote(t *testing.T, ref string, tarballPath string) {
	var cmd []string
	// test both paths because we want to test that the pulled tarball works as well
	t.Run(
		"deploy+remove bundle via OCI",
		func(t *testing.T) {
			cmd = strings.Split(fmt.Sprintf("bundle deploy oci://%s --insecure --oci-concurrency=10 --confirm", ref), " ")
			_, _, err := e2e.UDS(cmd...)
			require.NoError(t, err)

			cmd = strings.Split(fmt.Sprintf("bundle remove oci://%s --confirm --insecure", ref), " ")
			_, _, err = e2e.UDS(cmd...)
			require.NoError(t, err)
		},
	)

	t.Run(
		"deploy+remove bundle via local tarball",
		func(t *testing.T) {
			cmd = strings.Split(fmt.Sprintf("bundle deploy %s --confirm", tarballPath), " ")
			_, _, err := e2e.UDS(cmd...)
			require.NoError(t, err)

			cmd = strings.Split(fmt.Sprintf("bundle remove %s --confirm --insecure", tarballPath), " ")
			_, _, err = e2e.UDS(cmd...)
			require.NoError(t, err)
		},
	)
}

func shasMatch(t *testing.T, path string, expected string) {
	actual, err := utils.GetSHA256OfFile(path)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func pull(t *testing.T, ref string, tarballPath string) {
	// todo: output somewhere other than build?
	cmd := strings.Split(fmt.Sprintf("bundle pull oci://%s -o build --insecure --oci-concurrency=10", ref), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)

	decompressed := "build/decompressed-bundle"
	defer e2e.CleanFiles(decompressed)

	cmd = []string{"tools", "archiver", "decompress", tarballPath, decompressed}
	_, _, err = e2e.UDS(cmd...)
	require.NoError(t, err)

	index := ocispec.Index{}
	b, err := os.ReadFile(filepath.Join(decompressed, "index.json"))
	require.NoError(t, err)
	err = json.Unmarshal(b, &index)
	require.NoError(t, err)

	require.Equal(t, 1, len(index.Manifests))

	blobsDir := filepath.Join(decompressed, "blobs", "sha256")

	for _, desc := range index.Manifests {
		sha := desc.Digest.Encoded()
		shasMatch(t, filepath.Join(blobsDir, sha), desc.Digest.Encoded())

		manifest := ocispec.Manifest{}
		b, err := os.ReadFile(filepath.Join(blobsDir, sha))
		require.NoError(t, err)
		err = json.Unmarshal(b, &manifest)
		require.NoError(t, err)

		for _, layer := range manifest.Layers {
			sha := layer.Digest.Encoded()
			path := filepath.Join(blobsDir, sha)
			if assert.FileExists(t, path) {
				shasMatch(t, path, layer.Digest.Encoded())
			} else {
				t.Logf("layer dne, but it might be part of a component that is not included in this bundle: \n %#+v", layer)
			}
		}
	}
}
