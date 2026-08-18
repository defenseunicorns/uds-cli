// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
)

// AdjacentDefaultsPath returns the optional defaults file next to bundleDir.
// Missing files are represented by an empty path; other filesystem errors are
// returned to the caller.
func AdjacentDefaultsPath(bundleDir string) (string, error) {
	defaultsPath := filepath.Join(bundleDir, BundleDefaultsFileName)
	if _, err := os.Stat(defaultsPath); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return defaultsPath, nil
}
