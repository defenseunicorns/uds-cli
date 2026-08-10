// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	godigest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// digestOf computes the OCI digest of test content.
func digestOf(data []byte) godigest.Digest {
	sum := sha256.Sum256(data)
	return godigest.NewDigestFromEncoded(godigest.SHA256, hex.EncodeToString(sum[:]))
}

func TestWriteBlobBytesIfMissingAndVerify(t *testing.T) {
	t.Run("writes blob and verifies digest", func(t *testing.T) {
		blobDir := t.TempDir()
		data := []byte("hello world")
		d := digestOf(data)

		err := writeBlobBytesIfMissingAndVerify(blobDir, d, data)
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(blobDir, d.Encoded()))
		require.NoError(t, err)
		assert.Equal(t, data, got)
	})

	t.Run("skips if blob already exists", func(t *testing.T) {
		blobDir := t.TempDir()
		data := []byte("existing blob")
		d := digestOf(data)

		// Write it once
		require.NoError(t, writeBlobBytesIfMissingAndVerify(blobDir, d, data))
		// Write again — should be a no-op
		require.NoError(t, writeBlobBytesIfMissingAndVerify(blobDir, d, data))

		got, err := os.ReadFile(filepath.Join(blobDir, d.Encoded()))
		require.NoError(t, err)
		assert.Equal(t, data, got)
	})

	t.Run("returns error on digest mismatch", func(t *testing.T) {
		blobDir := t.TempDir()
		data := []byte("some data")
		wrongDigest := digestOf([]byte("different data"))

		err := writeBlobBytesIfMissingAndVerify(blobDir, wrongDigest, data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "digest mismatch")

		// File should not exist
		_, statErr := os.Stat(filepath.Join(blobDir, wrongDigest.Encoded()))
		assert.True(t, os.IsNotExist(statErr))
	})
}

func TestWriteBlobReaderIfMissingAndVerify(t *testing.T) {
	t.Run("writes blob from reader", func(t *testing.T) {
		blobDir := t.TempDir()
		data := []byte("streamed content")
		d := digestOf(data)

		err := writeBlobReaderIfMissingAndVerify(blobDir, d, bytes.NewReader(data))
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(blobDir, d.Encoded()))
		require.NoError(t, err)
		assert.Equal(t, data, got)
	})

	t.Run("skips if blob already exists", func(t *testing.T) {
		blobDir := t.TempDir()
		data := []byte("already here")
		d := digestOf(data)

		// Pre-write the blob
		require.NoError(t, os.WriteFile(filepath.Join(blobDir, d.Encoded()), data, 0o644))

		// Should skip without error
		err := writeBlobReaderIfMissingAndVerify(blobDir, d, bytes.NewReader([]byte("wrong content")))
		require.NoError(t, err)

		// Original content should be unchanged
		got, err := os.ReadFile(filepath.Join(blobDir, d.Encoded()))
		require.NoError(t, err)
		assert.Equal(t, data, got)
	})

	t.Run("returns error on digest mismatch", func(t *testing.T) {
		blobDir := t.TempDir()
		data := []byte("real data")
		wrongDigest := digestOf([]byte("expected data"))

		err := writeBlobReaderIfMissingAndVerify(blobDir, wrongDigest, bytes.NewReader(data))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "digest mismatch")
	})
}

func TestCopyBlobFileIfMissingAndVerify(t *testing.T) {
	t.Run("copies file as blob", func(t *testing.T) {
		srcDir := t.TempDir()
		dstDir := t.TempDir()
		data := []byte("file to copy")
		d := digestOf(data)

		srcPath := filepath.Join(srcDir, "source.bin")
		require.NoError(t, os.WriteFile(srcPath, data, 0o644))

		err := copyBlobFileIfMissingAndVerify(dstDir, srcPath, d)
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(dstDir, d.Encoded()))
		require.NoError(t, err)
		assert.Equal(t, data, got)
	})

	t.Run("returns error for missing source", func(t *testing.T) {
		dstDir := t.TempDir()
		d := digestOf([]byte("x"))

		err := copyBlobFileIfMissingAndVerify(dstDir, "/nonexistent/file", d)
		require.Error(t, err)
	})
}

func TestWriteAndDigestBlob(t *testing.T) {
	blobDir := t.TempDir()
	data := []byte("auto-digest content")

	d, err := writeAndDigestBlob(blobDir, data)
	require.NoError(t, err)

	// Verify returned digest is correct
	assert.Equal(t, digestOf(data), d)

	// Verify blob was written
	got, err := os.ReadFile(filepath.Join(blobDir, d.Encoded()))
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestGcUnreferencedBlobs(t *testing.T) {
	blobDir := t.TempDir()

	// Create a manifest with config + 1 layer
	configData := []byte(`{"architecture":"amd64"}`)
	configDigest := digestOf(configData)
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, configDigest.Encoded()), configData, 0o644))

	layerData := []byte("layer content")
	layerDigest := digestOf(layerData)
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, layerDigest.Encoded()), layerData, 0o644))

	im := ociImageManifest{
		SchemaVersion: 2,
		Config:        ociDescriptor{Digest: configDigest.String(), Size: int64(len(configData))},
		Layers:        []ociDescriptor{{Digest: layerDigest.String(), Size: int64(len(layerData))}},
	}
	manifestBytes, err := json.Marshal(im)
	require.NoError(t, err)
	manifestDigest := digestOf(manifestBytes)
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, manifestDigest.Encoded()), manifestBytes, 0o644))

	// Write an unreferenced blob
	orphanData := []byte("orphan blob")
	orphanDigest := digestOf(orphanData)
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, orphanDigest.Encoded()), orphanData, 0o644))

	manifests := []ociManifest{{Digest: manifestDigest.String(), Size: int64(len(manifestBytes))}}

	err = gcUnreferencedBlobs(t.Context(), iostreams.IOStreams{}, blobDir, manifests)
	require.NoError(t, err)

	// Referenced blobs should exist
	assert.FileExists(t, filepath.Join(blobDir, manifestDigest.Encoded()))
	assert.FileExists(t, filepath.Join(blobDir, configDigest.Encoded()))
	assert.FileExists(t, filepath.Join(blobDir, layerDigest.Encoded()))

	// Orphan should be removed
	_, statErr := os.Stat(filepath.Join(blobDir, orphanDigest.Encoded()))
	assert.True(t, os.IsNotExist(statErr), "orphan blob should have been removed")
}

func TestParseDigest(t *testing.T) {
	t.Run("valid digest", func(t *testing.T) {
		d, err := parseDigest("sha256:" + strings.Repeat("ab", 32))
		require.NoError(t, err)
		assert.Equal(t, godigest.SHA256, d.Algorithm())
	})

	t.Run("invalid digest", func(t *testing.T) {
		_, err := parseDigest("not-a-digest")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid digest")
	})
}
