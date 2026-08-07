// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeLayerDestinationPath(t *testing.T) {
	dstDir := t.TempDir()
	cleanDstDir, err := filepath.Abs(filepath.Clean(dstDir))
	require.NoError(t, err)

	tests := []struct {
		name        string
		title       string
		want        string
		wantErrFrag string
	}{
		{
			name:  "simple file",
			title: BundleFileName,
			want:  filepath.Join(dstDir, BundleFileName),
		},
		{
			name:  "nested file",
			title: "values/pkg/0.yaml",
			want:  filepath.Join(dstDir, "values", "pkg", "0.yaml"),
		},
		{
			name:  "cleaned internal traversal",
			title: "values/pkg/../pkg/0.yaml",
			want:  filepath.Join(dstDir, "values", "pkg", "0.yaml"),
		},
		{
			name:        "parent traversal rejected",
			title:       "../../../etc/passwd",
			wantErrFrag: "escapes destination directory",
		},
		{
			name:        "sibling prefix traversal rejected",
			title:       "../" + filepath.Base(dstDir) + "-sibling/file",
			wantErrFrag: "escapes destination directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeLayerDestinationPath(cleanDstDir, dstDir, tt.title)
			if tt.wantErrFrag != "" {
				require.ErrorContains(t, err, tt.wantErrFrag)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
