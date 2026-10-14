// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClearBundleCache(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cachePath := BundleCachePath(cacheDir)
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "keep")
	require.NoError(t, os.WriteFile(outsideFile, []byte("keep"), 0o600))
	for _, name := range []string{"bundle-oci/images/blobs/sha256/blob", "bundle-oci/images/index.json", "bundle-oci/images/ingest/partial", "layers/legacy", "images/zarf", "unrelated"} {
		path := filepath.Join(cacheDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("keep"), 0o600))
	}
	// Cleanup must remove links inside the cache without deleting their targets.
	require.NoError(t, os.Symlink(outside, filepath.Join(cachePath, "outside")))
	require.NoError(t, ClearBundleCache(t.Context(), cacheDir, iostreams.IOStreams{}))
	entries, err := os.ReadDir(cachePath)
	require.NoError(t, err)
	assert.Empty(t, entries)
	for _, name := range []string{"layers/legacy", "images/zarf", "unrelated"} {
		assert.FileExists(t, filepath.Join(cacheDir, name))
	}
	assert.FileExists(t, outsideFile)
	// An already-cleared cache is a successful no-op.
	require.NoError(t, ClearBundleCache(t.Context(), cacheDir, iostreams.IOStreams{}))
}

func TestClearBundleCacheMissing(t *testing.T) {
	t.Parallel()
	cacheDir := filepath.Join(t.TempDir(), "missing")
	require.NoError(t, ClearBundleCache(t.Context(), cacheDir, iostreams.IOStreams{}))
	assert.NoDirExists(t, cacheDir)
}

func TestClearBundleCacheRejectsInvalidTarget(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			cacheDir := t.TempDir()
			path := BundleCachePath(cacheDir)
			outside := t.TempDir()
			keep := filepath.Join(outside, "keep")
			require.NoError(t, os.WriteFile(keep, []byte("keep"), 0o600))
			if kind == "file" {
				require.NoError(t, os.WriteFile(path, []byte("keep"), 0o600))
			} else {
				require.NoError(t, os.Symlink(outside, path))
			}
			err := ClearBundleCache(t.Context(), cacheDir, iostreams.IOStreams{})
			require.ErrorContains(t, err, "must be a directory")
			assert.FileExists(t, keep)
			_, err = os.Lstat(path)
			require.NoError(t, err)
		})
	}
	var empty EmptyParameterError
	err := ClearBundleCache(t.Context(), "", iostreams.IOStreams{})
	require.ErrorAs(t, err, &empty)
}

func TestClearBundleCacheCancelled(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	keep := filepath.Join(BundleCachePath(cacheDir), "keep")
	require.NoError(t, os.MkdirAll(filepath.Dir(keep), 0o700))
	require.NoError(t, os.WriteFile(keep, nil, 0o600))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, ClearBundleCache(ctx, cacheDir, iostreams.IOStreams{}), context.Canceled)
	assert.FileExists(t, keep)
}
