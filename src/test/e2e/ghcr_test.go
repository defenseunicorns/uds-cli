// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"path/filepath"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry"
)

// NOTE: These tests need to have the string "GHCR" in their names
//       to ensure they are not run by the test-e2e-no-ghcr make target

func TestBundleDeployFromOCIFromGHCR(t *testing.T) {
	deployZarfInit(t)

	bundleName := "ghcr-test"
	bundleDir := "src/test/bundles/06-ghcr"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-ghcr-test-%s-0.0.1.tar.zst", e2e.Arch))

	registryURL := "oci://ghcr.io/defenseunicorns/packages/uds-cli/test/publish"
	bundleGHCRPath := "defenseunicorns/packages/uds-cli/test/publish"
	tarballPath := filepath.Join("build", fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch))
	bundleRef := registry.Reference{
		Registry: registryURL,
		// this info is derived from the bundle's metadata
		Repository: "ghcr-test",
		Reference:  fmt.Sprintf("0.0.1-%s", e2e.Arch),
	}

	createLocal(t, bundleDir)
	inspect(t, bundlePath)
	publish(t, bundlePath, registryURL)
	// test without oci prefix
	registryURL = "ghcr.io/defenseunicorns/packages/uds-cli/test/publish"
	publish(t, bundlePath, registryURL)
	pull(t, bundleRef.String(), tarballPath)
	deploy(t, bundleRef.String())
	remove(t, bundlePath)

	// ensure the bundle index is present
	index, err := queryIndex(t, "https://ghcr.io", fmt.Sprintf("%s/%s", bundleGHCRPath, bundleName))
	require.NoError(t, err)
	require.Equal(t, 1, len(index.Manifests))
	require.Equal(t, ocispec.MediaTypeImageIndex, index.MediaType)
	require.Equal(t, ocispec.MediaTypeImageManifest, index.Manifests[0].MediaType)
	require.Equal(t, e2e.Arch, index.Manifests[0].Platform.Architecture)
	require.Equal(t, "multi", index.Manifests[0].Platform.OS)
}

// test the create -o path
func TestBundleCreateAndDeployGHCR(t *testing.T) {
	deployZarfInit(t)

	bundleDir := "src/test/bundles/06-ghcr"
	bundleName := "ghcr-test"
	bundleGHCRPath := "defenseunicorns/packages/uds-cli/test/create-remote"
	registryURL := fmt.Sprintf("ghcr.io/%s", bundleGHCRPath)
	bundleRef := registry.Reference{
		Registry: registryURL,
		// this info is derived from the bundle's metadata
		Repository: "ghcr-test",
		Reference:  fmt.Sprintf("0.0.1-%s", e2e.Arch),
	}
	createRemoteSecure(t, bundleDir, registryURL)
	inspect(t, bundleRef.String())
	deploy(t, bundleRef.String())
	remove(t, bundleRef.String())

	// ensure the bundle index is present
	index, err := queryIndex(t, "https://ghcr.io", fmt.Sprintf("%s/%s", bundleGHCRPath, bundleName))
	require.NoError(t, err)
	require.Equal(t, 1, len(index.Manifests))
	require.Equal(t, ocispec.MediaTypeImageIndex, index.MediaType)
	require.Equal(t, ocispec.MediaTypeImageManifest, index.Manifests[0].MediaType)
	require.Equal(t, e2e.Arch, index.Manifests[0].Platform.Architecture)
	require.Equal(t, "multi", index.Manifests[0].Platform.OS)
}

// This test requires the following to be published (based on src/test/bundles/06-ghcr/uds-bundle.yaml):
// ghcr.io/defenseunicorns/packages/uds/bundles/ghcr-test:0.0.1-amd64
// ghcr.io/defenseunicorns/packages/uds/bundles/ghcr-test:0.0.1-arm64
// ghcr.io/defenseunicorns/packages/delivery/ghcr-test:0.0.1-amd64
// ghcr.io/defenseunicorns/packages/delivery/ghcr-test:0.0.1-arm64
// ghcr.io/defenseunicorns/packages/delivery/ghcr-delivery-test:0.0.1-amd64
// ghcr.io/defenseunicorns/packages/delivery/ghcr-delivery-test:0.0.1-arm64
// The default bundle location if no source path provided is defenseunicorns/packages/uds/bundles/"
func TestGHCRPathExpansion(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

	tarballPath := filepath.Join("build", fmt.Sprintf("uds-bundle-ghcr-test-%s-0.0.1.tar.zst", e2e.Arch))

	bundleName := "ghcr-test:0.0.1"
	inspect(t, bundleName)
	pull(t, bundleName, tarballPath)
	deploy(t, bundleName)
	remove(t, bundleName)

	bundleName = fmt.Sprintf("ghcr-delivery-test:0.0.1-%s", e2e.Arch)
	inspect(t, bundleName)
	pull(t, bundleName, tarballPath)
	deploy(t, bundleName)
	remove(t, bundleName)

	bundleName = fmt.Sprintf("delivery/ghcr-test:0.0.1-%s", e2e.Arch)
	inspect(t, bundleName)
	pull(t, bundleName, tarballPath)
	deploy(t, bundleName)
	remove(t, bundleName)

	bundleName = "ghcr.io/defenseunicorns/packages/delivery/ghcr-delivery-test:0.0.1"
	inspect(t, bundleName)
	pull(t, bundleName, tarballPath)
	deploy(t, bundleName)
	remove(t, bundleName)
}
