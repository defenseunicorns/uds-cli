// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// BundleCachePath returns the Next bundle cache path, or empty when caching is disabled.
func BundleCachePath(cacheDir string) string {
	if cacheDir == "" {
		return ""
	}
	return filepath.Join(cacheDir, "bundle-oci")
}

// PrepareBundleCache creates private Next cache directories before upstream Zarf
// opens its blob storage. Existing unsafe directories require operator repair;
// changing their permissions cannot undo prior exposure of private content.
// An empty cacheDir disables caching.
func PrepareBundleCache(cacheDir string) (string, error) {
	if cacheDir == "" {
		return "", nil
	}
	cacheDir = filepath.Clean(cacheDir)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", fmt.Errorf("creating cache directory: %w", err)
	}
	info, err := os.Lstat(cacheDir)
	if err != nil {
		return "", fmt.Errorf("checking cache directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cache root %q must be a directory, not a file or symlink", cacheDir)
	}
	root, err := os.OpenRoot(cacheDir)
	if err != nil {
		return "", fmt.Errorf("opening cache root: %w", err)
	}
	defer func() { _ = root.Close() }()
	opened, err := root.Stat(".")
	if err != nil {
		return "", fmt.Errorf("checking opened cache root: %w", err)
	}
	if !os.SameFile(info, opened) {
		return "", fmt.Errorf("cache root %q changed while opening", cacheDir)
	}
	// Prepare the directories used by upstream OCI storage, including all
	// supported SHA-2 digest algorithms. Zarf retains ownership of blob bytes.
	for _, name := range []string{
		"bundle-oci", "bundle-oci/images", "bundle-oci/images/blobs",
		"bundle-oci/images/blobs/sha256", "bundle-oci/images/blobs/sha384",
		"bundle-oci/images/blobs/sha512", "bundle-oci/images/ingest",
	} {
		if err := root.Mkdir(name, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("creating private cache directory %q: %w", name, err)
		}
		info, err := root.Lstat(name)
		if err != nil {
			return "", fmt.Errorf("checking private cache directory %q: %w", name, err)
		}
		if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
			return "", fmt.Errorf("cache directory %q must be a user-only directory (0700), not a file or symlink; repair permissions or remove the cache before retrying", filepath.Join(cacheDir, name))
		}
	}
	return BundleCachePath(cacheDir), nil
}

// ClearBundleCache removes only the children of the Next bundle cache directory.
// It preserves the root and sibling caches and does not follow content symlinks.
func ClearBundleCache(ctx context.Context, cacheDir string, streams iostreams.IOStreams) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if cacheDir == "" {
		streams.Debug("Next OCI cache disabled, skipping cleanup")
		return nil
	}
	cacheDir = filepath.Clean(cacheDir)
	parentInfo, err := os.Lstat(cacheDir)
	if errors.Is(err, os.ErrNotExist) {
		streams.Debug("Next OCI cache does not exist, skipping cleanup", "path", BundleCachePath(cacheDir))
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking cache root %q: %w", cacheDir, err)
	}
	if !parentInfo.IsDir() {
		return fmt.Errorf("cache root %q must be a directory, not a file or symlink", cacheDir)
	}
	path := BundleCachePath(cacheDir)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		streams.Debug("Next OCI cache does not exist, skipping cleanup", "path", path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking Next OCI cache %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("next OCI cache %q must be a directory, not a file or symlink", path)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return fmt.Errorf("opening Next OCI cache %q: %w", path, err)
	}
	defer func() { _ = root.Close() }()
	// Reject replacement between Lstat and OpenRoot, including a swapped symlink.
	opened, err := root.Stat(".")
	if err != nil {
		return fmt.Errorf("checking opened Next OCI cache %q: %w", path, err)
	}
	if !os.SameFile(info, opened) {
		return fmt.Errorf("next OCI cache %q changed while opening", path)
	}
	dir, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("opening Next OCI cache directory %q: %w", path, err)
	}
	entries, readErr := dir.ReadDir(-1)
	if err := errors.Join(readErr, dir.Close()); err != nil {
		return fmt.Errorf("listing Next OCI cache %q: %w", path, err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := root.RemoveAll(entry.Name()); err != nil {
			return fmt.Errorf("removing Next OCI cache entry %q: %w", filepath.Join(path, entry.Name()), err)
		}
		streams.Debug("removed Next OCI cache content", "path", filepath.Join(path, entry.Name()))
	}
	streams.Debug("Next OCI cache cleared", "path", path)
	return nil
}
