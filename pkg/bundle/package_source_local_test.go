// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// digestToHex extracts the hex portion from a digest string for test assertions.
func digestToHex(t *testing.T, digest string) string {
	t.Helper()
	parts := strings.SplitN(digest, ":", 2)
	require.Len(t, parts, 2, "invalid digest format: %s", digest)
	return parts[1]
}

func TestLocalSource_IngestFiltered_ZarfPackage(t *testing.T) {
	pkgDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "zarf.yaml"), []byte("metadata:\n  name: test\n  version: 1.0.0\ncomponents: []\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "checksums.txt"), []byte(""), 0o644))

	blobDir := t.TempDir()

	src := &localSource{path: pkgDir, arch: "amd64", bundleDir: ""}
	manifests, err := src.IngestFiltered(t.Context(), filters.Empty(), blobDir)
	require.NoError(t, err)
	assert.Len(t, manifests, 1)
	assert.Equal(t, "application/vnd.oci.image.manifest.v1+json", manifests[0].MediaType)
}

func TestLocalSource_IngestFiltered_OCILayout(t *testing.T) {
	layoutDir := t.TempDir()
	writeMinimalOCILayout(t, layoutDir)

	blobDir := t.TempDir()

	src := &localSource{path: layoutDir, arch: "amd64", bundleDir: ""}
	manifests, err := src.IngestFiltered(t.Context(), filters.Empty(), blobDir)
	require.NoError(t, err)
	assert.NotEmpty(t, manifests)
}

func TestLocalSource_IngestFiltered_RelativePath(t *testing.T) {
	bundleDir := t.TempDir()
	pkgDir := filepath.Join(bundleDir, "my-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "zarf.yaml"), []byte("metadata:\n  name: rel-test\n  version: 0.1.0\ncomponents: []\n"), 0o644))

	blobDir := t.TempDir()

	src := &localSource{path: "my-pkg", arch: "amd64", bundleDir: bundleDir}
	manifests, err := src.IngestFiltered(t.Context(), filters.Empty(), blobDir)
	require.NoError(t, err)
	assert.Len(t, manifests, 1)
}

func TestLocalSourcePullFilteredArchiveUsesCallerWorkspace(t *testing.T) {
	pkgDir := t.TempDir()
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	zarfYAML := "kind: ZarfPackageConfig\nmetadata:\n  name: test\n  version: 1.0.0\n  aggregateChecksum: " + emptySHA256 + "\ncomponents: []\n"
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "zarf.yaml"), []byte(zarfYAML), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "checksums.txt"), nil, 0o600))

	archivePath := filepath.Join(t.TempDir(), "zarf-package-test-amd64-1.0.0.tar.zst")
	require.NoError(t, writeTarZst(t.Context(), iostreams.IOStreams{}, archivePath, pkgDir))

	workspace := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, os.MkdirAll(workspace, tempDirPerm))
	src := &localSource{path: archivePath, arch: "amd64", streams: iostreams.IOStreams{}}
	pkgLayout, err := src.PullFiltered(t.Context(), workspace, layout.PackageLayoutOptions{
		Filter:               filters.Empty(),
		VerificationStrategy: layout.VerifyNever,
	})
	require.NoError(t, err)
	assert.Equal(t, workspace, pkgLayout.DirPath())

	require.NoError(t, pkgLayout.Cleanup())
	assert.NoDirExists(t, workspace)
}

func TestLocalSource_ResolvedPath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		bundleDir string
		want      string
	}{
		{
			name:      "absolute path unchanged",
			path:      "/abs/path/pkg",
			bundleDir: "/bundle",
			want:      "/abs/path/pkg",
		},
		{
			name:      "relative path joined with bundleDir",
			path:      "my-pkg",
			bundleDir: "/bundle/dir",
			want:      "/bundle/dir/my-pkg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &localSource{path: tt.path, bundleDir: tt.bundleDir}
			assert.Equal(t, tt.want, src.resolvedPath())
		})
	}
}

func TestIsZarfPackage(t *testing.T) {
	t.Run("returns true when zarf.yaml exists", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "zarf.yaml"), []byte("metadata:\n  name: test"), tmpFilePerm))

		got := isZarfPackage(root)
		assert.True(t, got)
	})

	t.Run("returns false when zarf.yaml does not exist", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "other.yaml"), []byte("test"), tmpFilePerm))

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
	ctx := t.Context()

	t.Run("successfully ingests simple Zarf package", func(t *testing.T) {
		// Create a minimal Zarf package
		pkgRoot := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(pkgRoot, "zarf.yaml"), []byte("metadata:\n  name: test\n  version: 1.0.0"), tmpFilePerm))
		require.NoError(t, os.WriteFile(filepath.Join(pkgRoot, "checksums.txt"), []byte("abc123"), tmpFilePerm))

		// Create components directory
		componentsDir := filepath.Join(pkgRoot, "components")
		require.NoError(t, os.MkdirAll(componentsDir, tempDirPerm))
		require.NoError(t, os.WriteFile(filepath.Join(componentsDir, "test.tar"), []byte("test component data"), tmpFilePerm))

		// Create blob directory
		blobDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(blobDir), tempDirPerm))

		manifests, err := ingestZarfPackage(ctx, iostreams.IOStreams{}, blobDir, pkgRoot, "amd64")
		require.NoError(t, err)
		require.Len(t, manifests, 1)

		// Verify manifest structure
		m := manifests[0]
		assert.Equal(t, "application/vnd.oci.image.manifest.v1+json", m.MediaType)
		assert.NotEmpty(t, m.Digest)
		assert.Positive(t, m.Size)

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
		require.NoError(t, os.WriteFile(filepath.Join(pkgRoot, "zarf.yaml"), []byte("test"), tmpFilePerm))

		// Create nested directory structure
		nestedDir := filepath.Join(pkgRoot, "a", "b", "c")
		require.NoError(t, os.MkdirAll(nestedDir, tempDirPerm))
		require.NoError(t, os.WriteFile(filepath.Join(nestedDir, "file.txt"), []byte("nested"), tmpFilePerm))

		blobDir := t.TempDir()
		require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

		manifests, err := ingestZarfPackage(ctx, iostreams.IOStreams{}, blobDir, pkgRoot, "amd64")
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
		require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

		_, err := ingestZarfPackage(ctx, iostreams.IOStreams{}, blobDir, pkgRoot, "arm64")
		require.Error(t, err)
		assert.ErrorContains(t, err, "no files found")
	})

	t.Run("skips symlinks", func(t *testing.T) {
		pkgRoot := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(pkgRoot, "zarf.yaml"), []byte("test"), tmpFilePerm))
		require.NoError(t, os.WriteFile(filepath.Join(pkgRoot, "target.txt"), []byte("target"), tmpFilePerm))

		// Create a symlink
		linkPath := filepath.Join(pkgRoot, "link.txt")
		require.NoError(t, os.Symlink("target.txt", linkPath))

		blobDir := t.TempDir()
		require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

		manifests, err := ingestZarfPackage(ctx, iostreams.IOStreams{}, blobDir, pkgRoot, "amd64")
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
		require.NoError(t, os.WriteFile(filepath.Join(pkgRoot, "zarf.yaml"), []byte("test"), tmpFilePerm))

		// Create executable file
		execPath := filepath.Join(pkgRoot, "script.sh")
		require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/bash\necho test"), 0o755))

		blobDir := t.TempDir()
		require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

		manifests, err := ingestZarfPackage(ctx, iostreams.IOStreams{}, blobDir, pkgRoot, "amd64")
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
