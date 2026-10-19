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
