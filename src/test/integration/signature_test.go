// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package integration

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestCreateSignedPackageRequiresVerificationMaterial(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	bundleDir := testutil.CopyFixture(t, "bundles/20-signed-no-key")

	created := runBundleCLI(t, workspace, "create", bundleDir, "--confirm", "--insecure", "--architecture", runtime.GOARCH)
	require.Error(t, created.Err)
	require.Contains(t, created.Stdout+created.Stderr, "failed to create bundle: package is signed but no verification material was provided")

	skipped := runBundleCLI(t, workspace, "create", bundleDir, "--confirm", "--insecure", "--skip-signature-validation", "--architecture", runtime.GOARCH, "--output", bundleDir)
	require.NoError(t, skipped.Err, skipped.Stderr)
}

func TestInspectSignedPackageWithoutVerificationMaterial(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	bundleDir := testutil.CopyFixture(t, "bundles/20-signed-no-key")
	created := runBundleCLI(t, workspace, "create", bundleDir, "--confirm", "--insecure", "--skip-signature-validation", "--architecture", runtime.GOARCH, "--output", bundleDir)
	require.NoError(t, created.Err, created.Stderr)
	bundlePath := filepath.Join(bundleDir, "uds-bundle-signed-no-key-"+runtime.GOARCH+"-0.0.1.tar.zst")

	for _, source := range []string{filepath.Join(bundleDir, config.BundleYAML), bundlePath} {
		source := source
		t.Run(filepath.Base(source), func(t *testing.T) {
			for _, extraArgs := range [][]string{nil, {"--list-images"}} {
				result := runBundleCLI(t, workspace, append([]string{"inspect", source}, extraArgs...)...)
				require.Error(t, result.Err)
				require.Contains(t, result.Stdout+result.Stderr, "failed to inspect bundle: package \"dos-games-no-key\": package is signed but no verification material was provided")
			}

			result := runBundleCLI(t, workspace, "inspect", source, "--skip-signature-validation")
			require.NoError(t, result.Err, result.Stderr)
		})
	}
}

func TestInspectSignedPackageWithPublicKey(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	bundleDir := testutil.CopyFixture(t, "bundles/21-signed-with-key")
	created := runBundleCLI(t, workspace, "create", bundleDir, "--confirm", "--insecure", "--skip-signature-validation", "--architecture", runtime.GOARCH, "--output", bundleDir)
	require.NoError(t, created.Err, created.Stderr)
	bundlePath := filepath.Join(bundleDir, "uds-bundle-signed-with-key-"+runtime.GOARCH+"-0.0.1.tar.zst")

	for _, source := range []string{filepath.Join(bundleDir, config.BundleYAML), bundlePath} {
		source := source
		t.Run(filepath.Base(source), func(t *testing.T) {
			inspected := runBundleCLI(t, workspace, "inspect", source)
			require.NoError(t, inspected.Err, inspected.Stderr)

			images := runBundleCLI(t, workspace, "inspect", source, "--list-images")
			require.NoError(t, images.Err, images.Stderr)
			require.Contains(t, images.Stdout, "dos-games")
		})
	}
}
