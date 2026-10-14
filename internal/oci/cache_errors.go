// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// IsCacheStorageError identifies filesystem failures attributable to Next's OCI
// cache. Upstream does not expose a cache-specific error type, so other failures
// (including unclassified index decoding errors) deliberately do not qualify.
func IsCacheStorageError(err error, cacheDir string) bool {
	if err == nil || cacheDir == "" || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	root, rootErr := filepath.Abs(BundleCachePath(cacheDir))
	base, baseErr := filepath.Abs(cacheDir)
	if rootErr != nil || baseErr != nil {
		return false
	}
	cachePath := func(path string) bool {
		path, pathErr := filepath.Abs(path)
		if pathErr != nil {
			return false
		}
		// Include the configured root itself: a file there can prevent the
		// bundle-oci directory from being created. Exclude sibling caches.
		return path == base || path == root || strings.HasPrefix(path, root+string(os.PathSeparator))
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return cachePath(pathErr.Path)
	}
	var linkErr *os.LinkError
	return errors.As(err, &linkErr) && cachePath(linkErr.Old) && cachePath(linkErr.New)
}
