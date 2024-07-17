// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBundleIndexInRemoteOnPublish(t *testing.T) {
	deployZarfInit(t)
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	bundleDir := "src/test/bundles/06-ghcr"
	bundleName := "ghcr-test"
	bundleTarballName := fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch)
	bundlePathARM := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, "arm64"))
	bundlePathAMD := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, "amd64"))
	tarballPath := filepath.Join("build", bundleTarballName)

	// create and push bundles with different archs to the same OCI repo
	createLocal(t, bundleDir, "arm64")
	createLocal(t, bundleDir, "amd64")
	publishInsecure(t, bundlePathARM, "localhost:888")
	publishInsecure(t, bundlePathAMD, "localhost:888")

	// curl OCI registry for index
	index, err := queryIndex(t, "http://localhost:888", bundleName)
	require.NoError(t, err)
	validateMultiArchIndex(t, index)

	inspectRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName))
	pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), bundleTarballName) // test no oci prefix
	deployAndRemoveLocalAndRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), tarballPath)

	// now test by running 'create -o' over the bundle that was published
	createRemoteInsecure(t, bundleDir, "oci://localhost:888", e2e.Arch)
	index, err = queryIndex(t, "http://localhost:888", bundleName)
	require.NoError(t, err)
	validateMultiArchIndex(t, index)
	inspectRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName))
	pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), bundleTarballName) // test no oci prefix
	deployAndRemoveLocalAndRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), tarballPath)
}

func TestBundleIndexInRemoteOnCreate(t *testing.T) {
	deployZarfInit(t)
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	bundleDir := "src/test/bundles/06-ghcr"
	bundleName := "ghcr-test"
	bundleTarballName := fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch)
	tarballPath := filepath.Join("build", bundleTarballName)

	// create and push bundles with different archs to the same OCI repo
	createRemoteInsecure(t, bundleDir, "oci://localhost:888", "arm64")
	createRemoteInsecure(t, bundleDir, "oci://localhost:888", "amd64")

	// curl OCI registry for index
	index, err := queryIndex(t, "http://localhost:888", bundleName)
	require.NoError(t, err)
	validateMultiArchIndex(t, index)

	inspectRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName))
	pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), bundleTarballName) // test no oci prefix
	deployAndRemoveLocalAndRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), tarballPath)

	// now test by publishing over the bundle that was created with 'create -o'
	createLocal(t, bundleDir, e2e.Arch)
	publishInsecure(t, tarballPath, "localhost:888")
	index, err = queryIndex(t, "http://localhost:888", bundleName)
	require.NoError(t, err)
	validateMultiArchIndex(t, index)
	inspectRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName))
	pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), bundleTarballName) // test no oci prefix
	deployAndRemoveLocalAndRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), tarballPath)
}
