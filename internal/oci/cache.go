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
