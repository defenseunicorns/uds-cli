// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config"
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

	runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, "arm64"))
	runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, "amd64"))
	runCmd(t, fmt.Sprintf("publish %s %s --oci-concurrency=10", bundlePathARM, registryURL))

	// test without oci prefix
	registryURL = "ghcr.io/defenseunicorns/packages/uds-cli/test/publish"
	runCmd(t, fmt.Sprintf("publish %s %s --oci-concurrency=10", bundlePathAMD, registryURL))

	inspectRemote(t, registryURL, bundleName, "0.0.1")
	pull(t, bundleRef.String(), bundleTarballName)
	runCmd(t, fmt.Sprintf("deploy %s --retries 1 --confirm", bundleRef.String()))
	runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundleRef.String()))

	// ensure the bundle index is present
	index, err := queryIndex(t, "https://ghcr.io", fmt.Sprintf("%s/%s", bundleGHCRPath, bundleName))
	require.NoError(t, err)
	ValidateMultiArchIndex(t, index)
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

	runCmd(t, fmt.Sprintf("create %s -o %s --confirm -a %s", bundleDir, registryURL, "arm64"))
	runCmd(t, fmt.Sprintf("create %s -o %s --confirm -a %s", bundleDir, registryURL, "amd64"))

	inspectRemote(t, registryURL, bundleName, bundleRef.Reference)
	runCmd(t, fmt.Sprintf("deploy %s --retries 1 --confirm", bundleRef.String()))
	runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundleRef.String()))

	// ensure the bundle index is present
	index, err := queryIndex(t, "https://ghcr.io", fmt.Sprintf("%s/%s", bundleGHCRPath, bundleName))
	require.NoError(t, err)
	ValidateMultiArchIndex(t, index)
}

// This test requires the following to be published (based on src/test/bundles/06-ghcr/uds-bundle.yaml):
// ghcr.io/defenseunicorns/packages/uds/bundles/ghcr-test:0.0.1
// ghcr.io/defenseunicorns/packages/delivery/ghcr-test:0.0.1
// ghcr.io/defenseunicorns/packages/delivery/ghcr-delivery-test:0.0.1
// The default bundle location if no source path provided is defenseunicorns/packages/uds/bundles/"
func TestGHCRPathExpansion(t *testing.T) {
	ref := "0.0.1"
	bundleName := "ghcr-test"

	// remove any existing sbom tar files
	_ = os.Remove(fmt.Sprintf("%s-%s", bundleName, config.BundleSBOMTar))

	// test shorthand for: ghcr.io/defenseunicorns/packages/uds/bundles/ghcr-test:0.0.1
	inspectRemote(t, "", bundleName, ref)

	// test shorthand for: ghcr.io/defenseunicorns/packages/delivery/ghcr-test:0.0.1
	inspectRemote(t, "delivery/", bundleName, ref)

	// test shorthand for: ghcr.io/defenseunicorns/packages/delivery/ghcr-delivery-test:0.0.1
	bundleName = "ghcr-delivery-test"
	inspectRemote(t, "", bundleName, ref)
}
