// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPullRemoteCacheFallback checks automatic retry through the public Pull API:
// a broken cache must still allow one call to return a verified, usable bundle.
// Recovery must leave the cache and configuration unchanged and clean both attempts.
func TestPullRemoteCacheFallback(t *testing.T) {
	for _, phase := range []string{"initialization", "population"} {
		t.Run(phase, func(t *testing.T) {
			fixture, ref, policy := publishSignedLibraryBundle(t, startLibraryRegistry(t))
			config := libraryRegistryConfig(fixture.Config)
			cacheDir := filepath.Join(t.TempDir(), "cache")
			blockedPath := filepath.Join(cacheDir, "bundle-oci", "images")
			if phase == "population" {
				blockedPath = filepath.Join(blockedPath, "ingest")
			}
			// Keep the registry healthy and block only the cache. A file
			// where a directory is needed fails even when tests run as root.
			require.NoError(t, os.MkdirAll(filepath.Dir(blockedPath), 0o700))
			require.NoError(t, os.WriteFile(blockedPath, []byte("cache obstruction"), 0o600))
			config.Options.CacheDir = cacheDir
			streams, _, out, errOut := iostreams.NewTestIOStreams()
			pulled, err := bundle.Pull(t.Context(), ref, t.TempDir(), bundle.PullOptions{
				Config: config, Verification: policy, Streams: streams,
			})
			require.NoError(t, err)
			require.NotNil(t, pulled)
			require.FileExists(t, pulled.OutputPath)
			assert.Equal(t, ref, pulled.OCIReference)
			assert.Empty(t, out.String())
			assert.Equal(t, 1, strings.Count(errOut.String(), "retrying pull once without caching"))
			assert.Equal(t, cacheDir, config.Options.CacheDir)
			contents, err := os.ReadFile(blockedPath)
			require.NoError(t, err)
			assert.Equal(t, "cache obstruction", string(contents))
			entries, err := os.ReadDir(config.Options.TmpDir)
			require.NoError(t, err)
			assert.Empty(t, entries, "both pull workspaces must be cleaned")

			// The returned archive must verify and supply package content
			// independently of the still-broken cache.
			require.NoError(t, bundle.Verify(t.Context(), bundle.VerifyOptions{Source: pulled.OutputPath, Policy: policy, Config: config, TmpDir: t.TempDir()}))
			prepared, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, pulled.OutputPath, t.TempDir(), runtime.GOARCH)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, prepared.Close()) })
			for i := range prepared.Bundle.Packages {
				pkg := &prepared.Bundle.Packages[i]
				loaded, err := prepared.Loader.LoadPackageLayout(t.Context(), pkg, t.TempDir(), bundle.ZarfPackageLayoutLoadOptions{})
				require.NoError(t, err)
				assert.Equal(t, pkg.Name, loaded.Layout.PackageDefinition.Metadata.Name)
			}

			// Disabling the cache on retry must not disable signature checks.
			_, wrongKey := libraryKeyPair(t)
			failedDir := t.TempDir()
			failed, err := bundle.Pull(t.Context(), ref, failedDir, bundle.PullOptions{Config: config, Verification: bundle.VerificationPolicy{PublicKey: wrongKey}})
			require.ErrorIs(t, err, bundle.ErrPullBundle)
			require.ErrorContains(t, err, "verifying bundle signature")
			assert.Nil(t, failed)
			files, err := os.ReadDir(failedDir)
			require.NoError(t, err)
			assert.Empty(t, files)
		})
	}
}

// TestPullCacheDoesNotRetryVerificationFailures checks the boundary of cache
// retries: an invalid or unreadable verification key must fail without retrying,
// even when the key's filesystem path is inside the configured cache directory.
func TestPullCacheDoesNotRetryVerificationFailures(t *testing.T) {
	fixture, ref, _ := publishSignedLibraryBundle(t, startLibraryRegistry(t))
	config := libraryRegistryConfig(fixture.Config)
	config.Options.CacheDir = t.TempDir()
	_, wrongKey := libraryKeyPair(t)
	for _, key := range []string{wrongKey, filepath.Join(config.Options.CacheDir, "missing-key.pub")} {
		t.Run(filepath.Base(key), func(t *testing.T) {
			streams, _, _, errOut := iostreams.NewTestIOStreams()
			targetDir := t.TempDir()
			result, err := bundle.Pull(t.Context(), ref, targetDir, bundle.PullOptions{
				Config: config, Verification: bundle.VerificationPolicy{PublicKey: key}, Streams: streams,
			})
			require.ErrorIs(t, err, bundle.ErrPullBundle)
			require.ErrorContains(t, err, "verifying bundle signature")
			assert.Nil(t, result)
			// A verification key inside the cache root can itself produce a
			// filesystem error. Its location must not trigger cache fallback.
			assert.NotContains(t, errOut.String(), "retrying pull once without caching")
			files, err := os.ReadDir(targetDir)
			require.NoError(t, err)
			assert.Empty(t, files)
		})
	}
}
