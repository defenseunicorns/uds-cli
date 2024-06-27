// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry"
)

// NOTE: These tests need to have the string "GHCR" in their names
//       to ensure they are not run by the test-e2e-no-ghcr make target
//       Also, these tests are run nightly and on releases, not on PRs

func TestBundleCreateAndPublishGHCR(t *testing.T) {
	deployZarfInit(t)

	bundleName := "ghcr-test"
	bundleDir := "src/test/bundles/06-ghcr"
	bundlePathARM := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-ghcr-test-%s-0.0.1.tar.zst", "arm64"))
	bundlePathAMD := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-ghcr-test-%s-0.0.1.tar.zst", "amd64"))

	registryURL := "oci://ghcr.io/defenseunicorns/packages/uds-cli/test/publish"
	bundleGHCRPath := "defenseunicorns/packages/uds-cli/test/publish"
	bundleTarballName := fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch)
	bundleRef := registry.Reference{
		Registry:   registryURL,
		Repository: "ghcr-test",
		Reference:  "0.0.1",
	}

	createLocal(t, bundleDir, "arm64")
	createLocal(t, bundleDir, "amd64")
	publish(t, bundlePathARM, registryURL)
	// test without oci prefix
	registryURL = "ghcr.io/defenseunicorns/packages/uds-cli/test/publish"
	publish(t, bundlePathAMD, registryURL)
	inspectRemote(t, bundlePathARM)
	pull(t, bundleRef.String(), bundleTarballName)
	deploy(t, bundleRef.String())
	remove(t, bundleRef.String())

	// ensure the bundle index is present
	index, err := queryIndex(t, "https://ghcr.io", fmt.Sprintf("%s/%s", bundleGHCRPath, bundleName))
	require.NoError(t, err)
	validateMultiArchIndex(t, index)
}

// test the create -o path
func TestBundleCreateRemoteAndDeployGHCR(t *testing.T) {
	deployZarfInit(t)

	bundleDir := "src/test/bundles/06-ghcr"
	bundleName := "ghcr-test"
	bundleGHCRPath := "defenseunicorns/packages/uds-cli/test/create-remote"
	registryURL := fmt.Sprintf("ghcr.io/%s", bundleGHCRPath)
	bundleRef := registry.Reference{
		Registry:   registryURL,
		Repository: "ghcr-test",
		Reference:  "0.0.1",
	}
	createRemote(t, bundleDir, registryURL, "arm64")
	createRemote(t, bundleDir, registryURL, "amd64")
	inspectRemote(t, bundleRef.String())
	deploy(t, bundleRef.String())
	remove(t, bundleRef.String())

	// ensure the bundle index is present
	index, err := queryIndex(t, "https://ghcr.io", fmt.Sprintf("%s/%s", bundleGHCRPath, bundleName))
	require.NoError(t, err)
	validateMultiArchIndex(t, index)
}

// This test requires the following to be published (based on src/test/bundles/06-ghcr/uds-bundle.yaml):
// ghcr.io/defenseunicorns/packages/uds/bundles/ghcr-test:0.0.1
// ghcr.io/defenseunicorns/packages/uds/bundles/ghcr-test:0.0.1
// ghcr.io/defenseunicorns/packages/delivery/ghcr-test:0.0.1
// ghcr.io/defenseunicorns/packages/delivery/ghcr-test:0.0.1
// ghcr.io/defenseunicorns/packages/delivery/ghcr-delivery-test:0.0.1
// ghcr.io/defenseunicorns/packages/delivery/ghcr-delivery-test:0.0.1
// The default bundle location if no source path provided is defenseunicorns/packages/uds/bundles/"
func TestGHCRPathExpansion(t *testing.T) {
	bundleName := "ghcr-test:0.0.1"
	inspectRemote(t, bundleName)

	bundleName = fmt.Sprintf("ghcr-delivery-test:0.0.1-%s", e2e.Arch)
	inspectRemote(t, bundleName)

	bundleName = fmt.Sprintf("delivery/ghcr-test:0.0.1-%s", e2e.Arch)
	inspectRemote(t, bundleName)

	bundleName = "ghcr.io/defenseunicorns/packages/delivery/ghcr-delivery-test:0.0.1"
	inspectRemote(t, bundleName)
}
