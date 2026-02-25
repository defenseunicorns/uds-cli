// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// digestToHex extracts the hex portion from a digest string for test assertions.
func digestToHex(t *testing.T, digest string) string {
	t.Helper()
	parts := strings.SplitN(digest, ":", 2)
	require.Len(t, parts, 2, "invalid digest format: %s", digest)
	return parts[1]
}

func TestIsZarfPackage(t *testing.T) {
	t.Run("returns true when zarf.yaml exists", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "zarf.yaml"), []byte("metadata:\n  name: test"), 0o644))

		got := isZarfPackage(root)
		assert.True(t, got)
	})

	t.Run("returns false when zarf.yaml does not exist", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "other.yaml"), []byte("test"), 0o644))

		got := isZarfPackage(root)
		assert.False(t, got)
	})

	t.Run("returns false for empty directory", func(t *testing.T) {
		root := t.TempDir()

		got := isZarfPackage(root)
		assert.False(t, got)
	})

	t.Run("returns false for non-existent directory", func(t *testing.T) {
		got := isZarfPackage("/nonexistent/path/that/does/not/exist")
		assert.False(t, got)
	})
}

func TestIngestZarfPackage(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully ingests simple Zarf package", func(t *testing.T) {
		// Create a minimal Zarf package
		pkgRoot := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(pkgRoot, "zarf.yaml"), []byte("metadata:\n  name: test\n  version: 1.0.0"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(pkgRoot, "checksums.txt"), []byte("abc123"), 0o644))

		// Create components directory
		componentsDir := filepath.Join(pkgRoot, "components")
		require.NoError(t, os.MkdirAll(componentsDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(componentsDir, "test.tar"), []byte("test component data"), 0o644))

		// Create blob directory
		blobDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(blobDir), 0o755))

		manifests, err := ingestZarfPackage(ctx, blobDir, pkgRoot, "amd64")
		require.NoError(t, err)
		require.Len(t, manifests, 1)

		// Verify manifest structure
		m := manifests[0]
		assert.Equal(t, "application/vnd.oci.image.manifest.v1+json", m.MediaType)
		assert.NotEmpty(t, m.Digest)
		assert.Greater(t, m.Size, int64(0))
		require.NotNil(t, m.Platform)
		assert.Equal(t, "amd64", m.Platform.Architecture)
		assert.Equal(t, "linux", m.Platform.OS)

		// Verify manifest blob was written
		manifestHex := digestToHex(t, m.Digest)
		manifestPath := filepath.Join(blobDir, manifestHex)
		assert.FileExists(t, manifestPath)

		// Read and verify manifest contains expected layers
		manifestData, err := os.ReadFile(manifestPath)
		require.NoError(t, err)

		var imageManifest ociImageManifest
		require.NoError(t, json.Unmarshal(manifestData, &imageManifest))

		// Should have 3 layers: zarf.yaml, checksums.txt, components/test.tar
		assert.Len(t, imageManifest.Layers, 3)

		// Verify layer annotations
		layerTitles := make([]string, len(imageManifest.Layers))
		for i, layer := range imageManifest.Layers {
			layerTitles[i] = layer.Annotations["org.opencontainers.image.title"]
		}
		assert.Contains(t, layerTitles, "zarf.yaml")
		assert.Contains(t, layerTitles, "checksums.txt")
		assert.Contains(t, layerTitles, "components/test.tar")

		// Verify config blob exists
		configHex := digestToHex(t, imageManifest.Config.Digest)
		configPath := filepath.Join(blobDir, configHex)
		assert.FileExists(t, configPath)

		// Verify all layer blobs exist
		for _, layer := range imageManifest.Layers {
			layerHex := digestToHex(t, layer.Digest)
			layerPath := filepath.Join(blobDir, layerHex)
			assert.FileExists(t, layerPath)
		}
	})

	t.Run("uses forward slashes in layer titles", func(t *testing.T) {
		pkgRoot := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(pkgRoot, "zarf.yaml"), []byte("test"), 0o644))

		// Create nested directory structure
		nestedDir := filepath.Join(pkgRoot, "a", "b", "c")
		require.NoError(t, os.MkdirAll(nestedDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(nestedDir, "file.txt"), []byte("nested"), 0o644))

		blobDir := t.TempDir()
		require.NoError(t, os.MkdirAll(blobDir, 0o755))

		manifests, err := ingestZarfPackage(ctx, blobDir, pkgRoot, "amd64")
		require.NoError(t, err)

		// Read manifest and check layer titles
		manifestHex := digestToHex(t, manifests[0].Digest)
		manifestData, err := os.ReadFile(filepath.Join(blobDir, manifestHex))
		require.NoError(t, err)

		var imageManifest ociImageManifest
		require.NoError(t, json.Unmarshal(manifestData, &imageManifest))

		// Find the nested file layer
		var foundNestedTitle string
		for _, layer := range imageManifest.Layers {
			if title := layer.Annotations["org.opencontainers.image.title"]; title != "zarf.yaml" {
				foundNestedTitle = title
				break
			}
		}
		// Should use forward slashes regardless of OS
		assert.Equal(t, "a/b/c/file.txt", foundNestedTitle)
	})

	t.Run("returns error for empty package", func(t *testing.T) {
		pkgRoot := t.TempDir()
		// No files in package

		blobDir := t.TempDir()
		require.NoError(t, os.MkdirAll(blobDir, 0o755))

		_, err := ingestZarfPackage(ctx, blobDir, pkgRoot, "arm64")
		require.Error(t, err)
		assert.ErrorContains(t, err, "no files found")
	})

	t.Run("skips symlinks", func(t *testing.T) {
		pkgRoot := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(pkgRoot, "zarf.yaml"), []byte("test"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(pkgRoot, "target.txt"), []byte("target"), 0o644))

		// Create a symlink
		linkPath := filepath.Join(pkgRoot, "link.txt")
		require.NoError(t, os.Symlink("target.txt", linkPath))

		blobDir := t.TempDir()
		require.NoError(t, os.MkdirAll(blobDir, 0o755))

		manifests, err := ingestZarfPackage(ctx, blobDir, pkgRoot, "amd64")
		require.NoError(t, err)

		// Read manifest
		manifestHex := digestToHex(t, manifests[0].Digest)
		manifestData, err := os.ReadFile(filepath.Join(blobDir, manifestHex))
		require.NoError(t, err)

		var imageManifest ociImageManifest
		require.NoError(t, json.Unmarshal(manifestData, &imageManifest))

		// Should only have 2 layers: zarf.yaml and target.txt (symlink skipped)
		assert.Len(t, imageManifest.Layers, 2)

		layerTitles := make([]string, len(imageManifest.Layers))
		for i, layer := range imageManifest.Layers {
			layerTitles[i] = layer.Annotations["org.opencontainers.image.title"]
		}
		assert.Contains(t, layerTitles, "zarf.yaml")
		assert.Contains(t, layerTitles, "target.txt")
		assert.NotContains(t, layerTitles, "link.txt")
	})

	t.Run("preserves file permissions", func(t *testing.T) {
		pkgRoot := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(pkgRoot, "zarf.yaml"), []byte("test"), 0o644))

		// Create executable file
		execPath := filepath.Join(pkgRoot, "script.sh")
		require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/bash\necho test"), 0o755))

		blobDir := t.TempDir()
		require.NoError(t, os.MkdirAll(blobDir, 0o755))

		manifests, err := ingestZarfPackage(ctx, blobDir, pkgRoot, "amd64")
		require.NoError(t, err)

		// Read manifest
		manifestHex := digestToHex(t, manifests[0].Digest)
		manifestData, err := os.ReadFile(filepath.Join(blobDir, manifestHex))
		require.NoError(t, err)

		var imageManifest ociImageManifest
		require.NoError(t, json.Unmarshal(manifestData, &imageManifest))

		// Find the script layer
		var scriptLayer *ociDescriptor
		for i, layer := range imageManifest.Layers {
			if layer.Annotations["org.opencontainers.image.title"] == "script.sh" {
				scriptLayer = &imageManifest.Layers[i]
				break
			}
		}
		require.NotNil(t, scriptLayer, "script.sh layer not found")

		// Verify permissions are preserved
		mode := scriptLayer.Annotations["org.defenseunicorns.zarf.file.mode"]
		assert.Equal(t, "755", mode)
	})
}
