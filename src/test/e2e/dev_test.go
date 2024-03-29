// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestDevDeployWithLocalAndRemotePkgs(t *testing.T) {

	removeZarfInit()

	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

	bundleDir := "src/test/bundles/03-local-and-remote"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

	devDeploy(t, bundleDir)
	remove(t, bundlePath)
}

func TestDevDeployWithCreateLocalPkgs(t *testing.T) {

	removeZarfInit()

	e2e.DeleteZarfPkg(t, "src/test/packages/podinfo")

	bundleDir := "src/test/bundles/03-local-and-remote"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

	devDeploy(t, bundleDir)
	remove(t, bundlePath)
}
