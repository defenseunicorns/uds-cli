// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateTFBundle(t *testing.T) {
	t.Run("Test creating tf bundles with remote Zarf packages", func(t *testing.T) {
		bundleDir := "src/test/bundles/17-tf-bundle-remote"
		bundleTarballName := fmt.Sprintf("uds-bundle-tf-demo-bundle-remote-%s-0.0.1.tar.zst", e2e.Arch)
		bundlePath := filepath.Join(bundleDir, bundleTarballName)

		runCmd(t, fmt.Sprintf("dev tofu-create %s", bundleDir))
		_, err := os.Stat(bundlePath)
		require.NoError(t, err)
	})

	t.Run("Test creating tf bundles with local Zarf packages", func(t *testing.T) {
		bundleDir := "src/test/bundles/16-tf-bundle-local"
		bundleTarballName := fmt.Sprintf("uds-bundle-tf-demo-bundle-local-%s-0.0.1.tar.zst", e2e.Arch)
		bundlePath := filepath.Join(bundleDir, bundleTarballName)

		// Ensure the local zarf packages exist
		podinfoZarfPkgPath := "src/test/packages/podinfo"
		nginxZarfPkgPath := "src/test/packages/nginx"
		e2e.CreateZarfPkg(t, podinfoZarfPkgPath, false)
		e2e.CreateZarfPkg(t, nginxZarfPkgPath, false)

		runCmd(t, fmt.Sprintf("dev tofu-create %s", bundleDir))
		_, err := os.Stat(bundlePath)
		require.NoError(t, err)
	})
}

func TestPushPullTFBundle(t *testing.T) {
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	bundleDir := "src/test/bundles/17-tf-bundle-remote"
	bundleTarballName := fmt.Sprintf("uds-bundle-tf-demo-bundle-remote-%s-0.0.1.tar.zst", e2e.Arch)
	bundlePath := filepath.Join(bundleDir, bundleTarballName)

	runCmd(t, fmt.Sprintf("dev tofu-create %s", bundleDir))
	runCmd(t, fmt.Sprintf("publish %s %s --insecure", bundlePath, "oci://localhost:888"))
	defer os.Remove(bundlePath)

	remoteBundlePath := "oci://localhost:888/tf-demo-bundle-remote:0.0.1"
	pulledBundlePath := filepath.Join("build", bundleTarballName)
	runCmd(t, fmt.Sprintf("pull %s -o build/ --insecure", remoteBundlePath))
	_, err := os.Stat(pulledBundlePath)
	defer os.Remove(pulledBundlePath)
	require.NoError(t, err)
}
