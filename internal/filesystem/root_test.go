// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenRootFromLstatRejectsReplacement(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	path := filepath.Join(parent, "root")
	require.NoError(t, os.Mkdir(path, PrivateDirectoryMode))
	info, err := os.Lstat(path)
	require.NoError(t, err)
	require.NoError(t, os.Rename(path, filepath.Join(parent, "original")))
	require.NoError(t, os.Mkdir(path, PrivateDirectoryMode))

	root, err := OpenRootFromLstat(path, info)
	if root != nil {
		t.Cleanup(func() { require.NoError(t, root.Close()) })
	}
	require.ErrorContains(t, err, "changed while opening")
	require.Nil(t, root)
}
