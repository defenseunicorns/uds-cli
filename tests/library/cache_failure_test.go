// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRemoteCacheFailuresRecoverWithoutCache(t *testing.T) {
	for _, phase := range []string{"initialization", "population"} {
		t.Run(phase, func(t *testing.T) {
			packagePath := createLibraryPackage(t, t.TempDir(), "cache-failure")
			ref := "oci://" + publishLibraryPackage(t, packagePath, startLibraryRegistry(t))
			root := t.TempDir()
			bundleFile := filepath.Join(root, libraryBundleFileName)
			require.NoError(t, os.WriteFile(bundleFile, fmt.Appendf(nil, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "cache-failure"
  version = "1.0.0"
}
package "remote" {
  source = %q
  signature_verification { verify = false }
}
`, ref), 0o600))
			config := libraryRegistryConfig(libraryFixtureConfig(t))
			cacheDir, blockedPath := obstructLibraryCache(t, phase)
			config.Options.CacheDir = cacheDir

			// One create call must recover automatically: the cache remains
			// obstructed, but the uncached retry can retrieve the package.
			result, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
				Config:  config,
				Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.FileExists(t, result.OutputPath)
			assert.Equal(t, cacheDir, config.Options.CacheDir)
			contents, err := os.ReadFile(blockedPath)
			require.NoError(t, err)
			assert.Equal(t, "cache obstruction", string(contents))
			entries, err := os.ReadDir(config.Options.TmpDir)
			require.NoError(t, err)
			assert.Empty(t, entries, "both attempts must clean up their temporary workspaces")
		})
	}
}

// TestPullRemoteCacheFailuresRecoverWithoutCache checks that a single unsigned
// pull recovers by retrying without caching, just as remote package create does.
func TestPullRemoteCacheFailuresRecoverWithoutCache(t *testing.T) {
	for _, phase := range []string{"initialization", "population"} {
		t.Run(phase, func(t *testing.T) {
			fixture := createLibraryBundle(t)
			config := libraryRegistryConfig(fixture.Config)
			ref := startLibraryRegistry(t) + "/bundles/cache-failure:v1"
			_, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
			require.NoError(t, err)
			cacheDir, blockedPath := obstructLibraryCache(t, phase)
			config.Options.CacheDir = cacheDir
			targetDir := t.TempDir()

			// One pull call must handle the cache failure and uncached retry;
			// the caller must not have to change configuration and try again.
			result, err := bundle.Pull(t.Context(), ref, targetDir, bundle.PullOptions{
				Config:                    config,
				SkipSignatureVerification: true,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.FileExists(t, result.OutputPath)
			assert.Equal(t, cacheDir, config.Options.CacheDir)
			contents, err := os.ReadFile(blockedPath)
			require.NoError(t, err)
			assert.Equal(t, "cache obstruction", string(contents))
			entries, err := os.ReadDir(config.Options.TmpDir)
			require.NoError(t, err)
			assert.Empty(t, entries, "both attempts must clean up their temporary workspaces")
		})
	}
}

func obstructLibraryCache(t *testing.T, phase string) (string, string) {
	t.Helper()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	blockedPath := filepath.Join(cacheDir, "bundle-oci", "images")
	if phase == "population" {
		// A file at ingest/ lets the OCI store open normally but prevents
		// writing a downloaded blob's temporary file. Unlike chmod-based
		// tests, this fails deterministically even when run as root.
		blockedPath = filepath.Join(blockedPath, "ingest")
	}
	// A file at images/ prevents opening the OCI store in the first place.
	// All obstructions live in test-owned directories, never the user's cache.
	require.NoError(t, os.MkdirAll(filepath.Dir(blockedPath), 0o700))
	require.NoError(t, os.WriteFile(blockedPath, []byte("cache obstruction"), 0o600))
	return cacheDir, blockedPath
}
