// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package filesystem provides shared filesystem policies for repository-private code.
package filesystem

import "io/fs"

const (
	// PrivateDirectoryMode grants access only to the current user.
	PrivateDirectoryMode fs.FileMode = 0o700
	// PrivateFileMode grants read and write access only to the current user.
	PrivateFileMode fs.FileMode = 0o600
)
