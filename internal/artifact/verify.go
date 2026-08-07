// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// safeLayerDestinationPath returns the filesystem destination for an OCI layer
// title under dstDir and rejects titles that would escape dstDir.
// cleanDstDir must be filepath.Abs(filepath.Clean(dstDir)), pre-computed by the caller.
func safeLayerDestinationPath(cleanDstDir, dstDir, title string) (string, error) {
	dst := filepath.Join(dstDir, filepath.FromSlash(title))

	cleanDst, err := filepath.Abs(filepath.Clean(dst))
	if err != nil {
		return "", fmt.Errorf("resolving layer title %q: %w", title, err)
	}
	rel, err := filepath.Rel(cleanDstDir, cleanDst)
	if err != nil {
		return "", fmt.Errorf("checking layer title %q: %w", title, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("layer title %q escapes destination directory", title)
	}

	return dst, nil
}
