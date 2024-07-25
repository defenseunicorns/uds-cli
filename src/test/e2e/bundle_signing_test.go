// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/stretchr/testify/require"
)

func TestChecksumAndSignature(t *testing.T) {
	os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/09-uds-bundle-yml", "uds-config.yml"))
	defer os.Unsetenv("UDS_CONFIG")

	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/nginx", true)

	bundleDir := "src/test/bundles/09-uds-bundle-yml"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-yml-example-%s-0.0.1.tar.zst", e2e.Arch))
	privateKeyFlag := "--signing-key=src/test/e2e/bundle-test.prv-key"
	publicKeyFlag := "--key=src/test/e2e/bundle-test.pub"

	// Create bundle with private key
	runCmd(t, fmt.Sprintf("create %s --confirm %s", bundleDir, privateKeyFlag))

	// Inspect signed bundle with public key
	_, stderr := runCmd(t, fmt.Sprintf("inspect %s %s", bundlePath, publicKeyFlag))
	require.Contains(t, stderr, "Verified OK")

	// Inspect signed bundle without public key
	stdout, _, err := runCmdWithErr(fmt.Sprintf("inspect %s", bundlePath))
	require.NoError(t, err)
	require.Contains(t, stdout, lang.CmdBundleInspectSignedNoPublicKey)

	// Test that we get an error when trying to deploy a package without providing the public key
	_, stderr, err = runCmdWithErr(fmt.Sprintf("deploy %s --confirm", bundlePath))
	require.Error(t, err)
	require.Contains(t, stderr, "failed to validate bundle: package is signed, but no public key was provided")

	// Test that we get don't get an error when trying to deploy a package with a public key
	_, stderr = runCmd(t, fmt.Sprintf("deploy %s %s --confirm", bundlePath, publicKeyFlag))
	require.Contains(t, stderr, "Loaded bundled Zarf package: nginx")

	remove(t, bundlePath)
}
