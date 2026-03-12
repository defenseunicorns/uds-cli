// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterDescriptorsByArch(t *testing.T) {
	amd64 := &v1.Platform{OS: "linux", Architecture: "amd64"}
	arm64 := &v1.Platform{OS: "linux", Architecture: "arm64"}

	descs := []v1.Descriptor{
		{Platform: amd64},
		{Platform: arm64},
		{Platform: nil},
		{Platform: &v1.Platform{Architecture: ""}},
	}

	t.Run("empty arch returns all", func(t *testing.T) {
		got := filterDescriptorsByArch(descs, "")
		require.Equal(t, descs, got)
	})

	t.Run("amd64 keeps amd64 and nil-platform", func(t *testing.T) {
		got := filterDescriptorsByArch(descs, "amd64")
		require.Len(t, got, 3)
		assert.Equal(t, amd64, got[0].Platform)
		assert.Nil(t, got[1].Platform)
		assert.NotNil(t, got[2].Platform)
		assert.Empty(t, got[2].Platform.Architecture)
	})

	t.Run("arm64 keeps arm64 and nil-platform", func(t *testing.T) {
		got := filterDescriptorsByArch(descs, "arm64")
		require.Len(t, got, 3)
		assert.Equal(t, arm64, got[0].Platform)
	})

	t.Run("unknown arch returns only nil-platform entries", func(t *testing.T) {
		got := filterDescriptorsByArch(descs, "riscv64")
		require.Len(t, got, 2)
	})

	t.Run("no matches returns nil slice", func(t *testing.T) {
		onlyARM := []v1.Descriptor{{Platform: arm64}}
		got := filterDescriptorsByArch(onlyARM, "amd64")
		require.Empty(t, got)
	})
}

func TestFilterOCIManifestsByArch(t *testing.T) {
	amd64 := &specv1.Platform{OS: "linux", Architecture: "amd64"}
	arm64 := &specv1.Platform{OS: "linux", Architecture: "arm64"}

	manifests := []ociManifest{
		{Platform: amd64},
		{Platform: arm64},
		{Platform: nil},
		{Platform: &specv1.Platform{Architecture: ""}},
	}

	t.Run("empty arch returns all", func(t *testing.T) {
		got := filterOCIManifestsByArch(manifests, "")
		require.Equal(t, manifests, got)
	})

	t.Run("amd64 keeps amd64 and nil-platform", func(t *testing.T) {
		got := filterOCIManifestsByArch(manifests, "amd64")
		require.Len(t, got, 3)
		assert.Equal(t, amd64, got[0].Platform)
		assert.Nil(t, got[1].Platform)
	})

	t.Run("arm64 keeps arm64 and nil-platform", func(t *testing.T) {
		got := filterOCIManifestsByArch(manifests, "arm64")
		require.Len(t, got, 3)
		assert.Equal(t, arm64, got[0].Platform)
	})

	t.Run("no matches returns nil slice", func(t *testing.T) {
		onlyAMD := []ociManifest{{Platform: amd64}}
		got := filterOCIManifestsByArch(onlyAMD, "arm64")
		require.Empty(t, got)
	})
}

func TestFindOCILayoutRoot(t *testing.T) {
	t.Run("finds OCI layout in root directory", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "blobs", "sha256"), tempDirPerm))
		require.NoError(t, os.WriteFile(filepath.Join(root, "oci-layout"), []byte(`{"imageLayoutVersion": "1.0.0"}`), tmpFilePerm))
		require.NoError(t, os.WriteFile(filepath.Join(root, "index.json"), []byte(`{"schemaVersion": 2}`), tmpFilePerm))

		found, err := findOCILayoutRoot(root)
		require.NoError(t, err)
		assert.Equal(t, root, found)
	})

	t.Run("finds OCI layout in oci subdirectory", func(t *testing.T) {
		root := t.TempDir()
		ociDir := filepath.Join(root, "oci")
		require.NoError(t, os.MkdirAll(filepath.Join(ociDir, "blobs", "sha256"), tempDirPerm))
		require.NoError(t, os.WriteFile(filepath.Join(ociDir, "oci-layout"), []byte(`{"imageLayoutVersion": "1.0.0"}`), tmpFilePerm))
		require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), []byte(`{"schemaVersion": 2}`), tmpFilePerm))

		found, err := findOCILayoutRoot(root)
		require.NoError(t, err)
		assert.Equal(t, ociDir, found)
	})

	t.Run("finds OCI layout in images subdirectory", func(t *testing.T) {
		root := t.TempDir()
		imagesDir := filepath.Join(root, "images")
		require.NoError(t, os.MkdirAll(filepath.Join(imagesDir, "blobs", "sha256"), tempDirPerm))
		require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "oci-layout"), []byte(`{"imageLayoutVersion": "1.0.0"}`), tmpFilePerm))
		require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "index.json"), []byte(`{"schemaVersion": 2}`), tmpFilePerm))

		found, err := findOCILayoutRoot(root)
		require.NoError(t, err)
		assert.Equal(t, imagesDir, found)
	})

	t.Run("returns error when no OCI layout found", func(t *testing.T) {
		root := t.TempDir()
		// Create some random files but no OCI layout
		require.NoError(t, os.WriteFile(filepath.Join(root, "random.txt"), []byte("not an oci layout"), tmpFilePerm))

		found, err := findOCILayoutRoot(root)
		require.Error(t, err)
		require.ErrorContains(t, err, "no OCI image layout found")
		assert.Empty(t, found)
	})

	t.Run("prefers root over subdirectories", func(t *testing.T) {
		root := t.TempDir()

		// Create OCI layout in root
		require.NoError(t, os.MkdirAll(filepath.Join(root, "blobs", "sha256"), tempDirPerm))
		require.NoError(t, os.WriteFile(filepath.Join(root, "oci-layout"), []byte(`{"imageLayoutVersion": "1.0.0"}`), tmpFilePerm))
		require.NoError(t, os.WriteFile(filepath.Join(root, "index.json"), []byte(`{"schemaVersion": 2}`), tmpFilePerm))

		// Also create OCI layout in subdirectories (should be ignored)
		ociDir := filepath.Join(root, "oci")
		require.NoError(t, os.MkdirAll(filepath.Join(ociDir, "blobs", "sha256"), tempDirPerm))
		require.NoError(t, os.WriteFile(filepath.Join(ociDir, "oci-layout"), []byte(`{"imageLayoutVersion": "1.0.0"}`), tmpFilePerm))
		require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), []byte(`{"schemaVersion": 2}`), tmpFilePerm))

		found, err := findOCILayoutRoot(root)
		require.NoError(t, err)
		assert.Equal(t, root, found)
	})
}
