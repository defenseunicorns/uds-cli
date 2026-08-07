// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundlehcl

import (
	"os"
	"path/filepath"
)

// ResolveBundlePath resolves a user-provided bundle reference to the path of
// the bundle.uds.hcl file. If ref is a directory, the bundle file inside it is
// returned; otherwise ref is returned as-is.
//
// Assumes the path has already been validated with ValidateBundlePath.
func ResolveBundlePath(ref string) string {
	info, err := os.Stat(ref)
	if err != nil {
		return ref
	}
	if info.IsDir() {
		return filepath.Join(ref, BundleFileName)
	}
	return ref
}
