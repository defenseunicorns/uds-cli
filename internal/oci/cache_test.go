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

func TestPrepareBundleCache(t *testing.T) {
	t.Parallel()
	path, err := PrepareBundleCache("")
	require.NoError(t, err)
	assert.Empty(t, path)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	for range 2 {
		path, err = PrepareBundleCache(cacheDir)
		require.NoError(t, err)
		assert.Equal(t, BundleCachePath(cacheDir), path)
	}
	require.NoError(t, filepath.WalkDir(cacheDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), path)
		return nil
	}))
}

func TestPrepareBundleCacheRejectsUnsafeDirectories(t *testing.T) {
	t.Parallel()
	for _, name := range []string{".", "bundle-oci", "bundle-oci/images", "bundle-oci/images/blobs", "bundle-oci/images/blobs/sha256", "bundle-oci/images/ingest"} {
		for _, kind := range []string{"symlink", "file", "readable", "writable"} {
			if name == "." && (kind == "readable" || kind == "writable") {
				continue // Caller-owned parent permissions are outside the private subtree.
			}
			t.Run(name+"/"+kind, func(t *testing.T) {
				t.Parallel()
				cacheDir := filepath.Join(t.TempDir(), "cache")
				path := filepath.Join(cacheDir, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
				outside := t.TempDir()
				switch kind {
				case "symlink":
					require.NoError(t, os.Symlink(outside, path))
				case "file":
					require.NoError(t, os.WriteFile(path, nil, 0o600))
				default:
					require.NoError(t, os.Mkdir(path, 0o700))
					mode := os.FileMode(0o755)
					if kind == "writable" {
						mode = 0o722
					}
					require.NoError(t, os.Chmod(path, mode))
				}
				_, err := PrepareBundleCache(cacheDir)
				require.Error(t, err)
				entries, err := os.ReadDir(outside)
				require.NoError(t, err)
				assert.Empty(t, entries)
			})
		}
	}
}

func TestBundleCachePath(t *testing.T) {
	t.Parallel()
	assert.Empty(t, BundleCachePath(""))
	assert.Equal(t, filepath.Join("cache", "bundle-oci"), BundleCachePath("cache"))
}

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
	// Both top-level and nested links must be removed without following targets.
	for _, name := range []string{"outside", "images/outside"} {
		require.NoError(t, os.Symlink(outside, filepath.Join(cachePath, name)))
	}
	require.NoError(t, os.Symlink(outsideFile, filepath.Join(cachePath, "file-link")))
	require.NoError(t, os.Link(outsideFile, filepath.Join(cachePath, "hard-link")))
	require.NoError(t, ClearBundleCache(t.Context(), cacheDir, iostreams.IOStreams{}))
	entries, err := os.ReadDir(cachePath)
	require.NoError(t, err)
	assert.Empty(t, entries)
	for _, name := range []string{"layers/legacy", "images/zarf", "unrelated"} {
		assert.FileExists(t, filepath.Join(cacheDir, name))
	}
	content, err := os.ReadFile(outsideFile)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(content))
	require.NoError(t, ClearBundleCache(t.Context(), cacheDir, iostreams.IOStreams{}))
}

func TestClearBundleCacheMissingAndDisabled(t *testing.T) {
	t.Parallel()
	cacheDir := filepath.Join(t.TempDir(), "missing")
	require.NoError(t, ClearBundleCache(t.Context(), cacheDir, iostreams.IOStreams{}))
	assert.NoDirExists(t, cacheDir)
	require.NoError(t, ClearBundleCache(t.Context(), "", iostreams.IOStreams{}))
}

func TestClearBundleCacheRejectsInvalidTarget(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"file", "symlink", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			cacheDir := t.TempDir()
			path := BundleCachePath(cacheDir)
			outside := t.TempDir()
			keep := filepath.Join(outside, "keep")
			require.NoError(t, os.WriteFile(keep, []byte("keep"), 0o600))
			switch kind {
			case "file":
				require.NoError(t, os.WriteFile(path, []byte("keep"), 0o600))
			case "symlink":
				require.NoError(t, os.Symlink(outside, path))
			case "dangling symlink":
				require.NoError(t, os.Symlink(filepath.Join(outside, "missing"), path))
			}
			err := ClearBundleCache(t.Context(), cacheDir, iostreams.IOStreams{})
			require.ErrorContains(t, err, "must be a directory")
			assert.FileExists(t, keep)
			_, err = os.Lstat(path)
			require.NoError(t, err)
		})
	}
}

func TestCacheRejectsSymlinkRootWithTrailingSeparator(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	keep := filepath.Join(BundleCachePath(outside), "keep")
	require.NoError(t, os.MkdirAll(filepath.Dir(keep), 0o700))
	require.NoError(t, os.WriteFile(keep, nil, 0o600))
	link := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, os.Symlink(outside, link))
	for _, path := range []string{link, link + string(os.PathSeparator)} {
		_, err := PrepareBundleCache(path)
		require.ErrorContains(t, err, "not a file or symlink")
		require.ErrorContains(t, ClearBundleCache(t.Context(), path, iostreams.IOStreams{}), "not a file or symlink")
	}
	assert.FileExists(t, keep)
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
