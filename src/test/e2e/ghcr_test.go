// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"path/filepath"
	"testing"

	"oras.land/oras-go/v2/registry"
)

func TestBundleDeployFromOCIFromGHCR(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo")

	bundleDir := "src/test/bundles/06-ghcr"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-ghcr-test-%s-0.0.1.tar.zst", e2e.Arch))

	registryURL := "oci://ghcr.io/defenseunicorns/packages/uds/bundles/uds-cli/test"

	tarballPath := filepath.Join("build", fmt.Sprintf("uds-bundle-ghcr-test-%s-0.0.1.tar.zst", e2e.Arch))
	bundleRef := registry.Reference{
		Registry: registryURL,
		// this info is derived from the bundle's metadata
		Repository: "ghcr-test",
		Reference:  fmt.Sprintf("0.0.1-%s", e2e.Arch),
	}

	createSecure(t, bundleDir)
	inspect(t, bundlePath)
	publish(t, bundlePath, registryURL)
	pull(t, bundleRef.String(), tarballPath)
	deploy(t, bundleRef.String())
	remove(t, bundlePath)
}

// This test requires the following to be published (based on src/test/bundles/06-ghcr/uds-bundle.yaml):
// ghcr.io/defenseunicorns/packages/uds/bundles/ghcr-test:0.0.1-amd64
// ghcr.io/defenseunicorns/packages/uds/bundles/ghcr-test:0.0.1-arm64
// ghcr.io/defenseunicorns/packages/delivery/ghcr-test:0.0.1-amd64
// ghcr.io/defenseunicorns/packages/delivery/ghcr-test:0.0.1-arm64
// The default bundle location if no source path provided is defenseunicorns/packages/uds/bundles/"
func TestGHCRPathExpansion(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo")

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
}
