// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
)

// OpenRoot opens a directory whose final path element is not a symlink.
// The caller owns the returned root and must close it.
func OpenRoot(path string) (*os.Root, error) {
	if path == "" {
		return nil, fmt.Errorf("directory path is required")
	}
	// A trailing separator can make Lstat follow the final symlink.
	path = filepath.Clean(path)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	return OpenRootFromLstat(path, info)
}

// OpenRootFromLstat verifies the opened directory against info from an Lstat of
// the same cleaned path. Use it when the caller must handle lookup errors before
// opening the directory. The caller must close the returned root on success.
func OpenRootFromLstat(path string, info os.FileInfo) (*os.Root, error) {
	if !info.IsDir() {
		return nil, fmt.Errorf("path %q must be a directory, not a file or symlink", path)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	if !os.SameFile(info, opened) {
		_ = root.Close()
		return nil, fmt.Errorf("directory %q changed while opening", path)
	}
	return root, nil
}
