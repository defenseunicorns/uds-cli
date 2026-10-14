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

// ClearBundleCache removes Next cache content while preserving its directory
// and sibling caches. It does not coordinate with readers or writers; callers
// must ensure no other operation is using this cache during cleanup.
func ClearBundleCache(ctx context.Context, cacheDir string, streams iostreams.IOStreams) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if cacheDir == "" {
		return EmptyParameterError{Name: "cache directory"}
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
	// Do not follow a cache-directory symlink into unrelated content. Symlinks
	// inside the cache are removed as links, not traversed to their targets.
	if !info.IsDir() {
		return fmt.Errorf("next OCI cache %q must be a directory, not a file or symlink", path)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return fmt.Errorf("opening Next OCI cache %q: %w", path, err)
	}
	defer func() { _ = root.Close() }()
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
		// Root bounds each recursive deletion to the opened cache directory.
		// Remove children, not the root, so cleanup never removes <uds-cache>.
		if err := root.RemoveAll(entry.Name()); err != nil {
			return fmt.Errorf("removing Next OCI cache entry %q: %w", filepath.Join(path, entry.Name()), err)
		}
		streams.Debug("removed Next OCI cache content", "path", filepath.Join(path, entry.Name()))
	}
	streams.Debug("Next OCI cache cleared", "path", path)
	return nil
}
