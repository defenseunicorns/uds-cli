// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBundlePath(t *testing.T) {
	tempDir := t.TempDir()

	validDir := filepath.Join(tempDir, "valid")
	require.NoError(t, os.Mkdir(validDir, 0o755))
	validBundleFile := filepath.Join(validDir, BundleFileName)
	require.NoError(t, os.WriteFile(validBundleFile, []byte("test content"), 0o600))

	tests := []struct {
		name     string
		ref      string
		wantPath string
	}{
		{
			name:     "file path returns as-is",
			ref:      validBundleFile,
			wantPath: validBundleFile,
		},
		{
			name:     "directory path resolves to bundle file",
			ref:      validDir,
			wantPath: validBundleFile,
		},
		{
			name:     "non-existent path returns as-is",
			ref:      "/nonexistent/path",
			wantPath: "/nonexistent/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveBundlePath(tt.ref)
			assert.Equal(t, tt.wantPath, got)
		})
	}
}

func TestResolveBundlePath_WithRealBundle(t *testing.T) {
	bundleDir := filepath.Join("..", "..", "tests", "test_data", "bundles", "spec-compliant")
	bundleFile := filepath.Join(bundleDir, BundleFileName)

	t.Run("resolve directory to bundle file", func(t *testing.T) {
		got := ResolveBundlePath(bundleDir)
		assert.Equal(t, bundleFile, got)
	})

	t.Run("resolve file directly", func(t *testing.T) {
		got := ResolveBundlePath(bundleFile)
		assert.Equal(t, bundleFile, got)
	})
}
