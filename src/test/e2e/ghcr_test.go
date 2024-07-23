// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config/lang"
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
	createRemote(t, bundleDir, registryURL, "arm64")
	createRemote(t, bundleDir, registryURL, "amd64")
	inspectRemote(t, bundleRef.String())
	deploy(t, bundleRef.String())
	remove(t, bundleRef.String())

	// ensure the bundle index is present
	index, err := queryIndex(t, "https://ghcr.io", fmt.Sprintf("%s/%s", bundleGHCRPath, bundleName))
	require.NoError(t, err)
	ValidateMultiArchIndex(t, index)
}

// test the create -o path
func TestBundleCreateSignedRemoteAndDeployGHCR(t *testing.T) {
	deployZarfInit(t)

	bundleDir := "src/test/bundles/06-ghcr"
	bundleGHCRPath := "defenseunicorns/packages/uds-cli/test/create-signed-remote"
	privateKeyFlag := "--signing-key=src/test/e2e/bundle-test.prv-key"
	publicKeyFlag := "--key=src/test/e2e/bundle-test.pub"
	registryURL := fmt.Sprintf("ghcr.io/%s", bundleGHCRPath)
	bundleRef := registry.Reference{
		Registry:   registryURL,
		Repository: "ghcr-test",
		Reference:  "0.0.1",
	}

	// create arm64 bundle with private key
	cmd := strings.Split(fmt.Sprintf("create %s -o %s %s --confirm -a %s", bundleDir, registryURL, privateKeyFlag, "arm64"), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)

	// create amd64 bundle with private key
	cmd = strings.Split(fmt.Sprintf("create %s -o %s %s --confirm -a %s", bundleDir, registryURL, privateKeyFlag, "amd64"), " ")
	_, _, err = e2e.UDS(cmd...)
	require.NoError(t, err)

	// inspect signed bundle with public key
	cmd = strings.Split(fmt.Sprintf("inspect %s %s", bundleRef.String(), publicKeyFlag), " ")
	_, stderr, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	require.Contains(t, stderr, "Verified OK")

	// inspect signed bundle without public key
	cmd = strings.Split(fmt.Sprintf("inspect %s", bundleRef.String()), " ")
	stdout, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	require.Contains(t, stdout, lang.CmdBundleInspectSignedNoPublicKey)

	// Test that we get an error when trying to deploy a package without providing the public key
	_, stderr, err = runCmdWithErr(fmt.Sprintf("deploy %s --confirm", bundleRef.String()))
	require.Error(t, err)
	require.Contains(t, stderr, "failed to validate bundle: package is signed, but no public key was provided")

	// Test that we get don't get an error when trying to deploy a package with a public key
	_, stderr = runCmd(t, fmt.Sprintf("deploy %s %s --confirm", bundleRef.String(), publicKeyFlag))
	require.Contains(t, stderr, "succeeded")

	remove(t, bundleRef.String())
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
