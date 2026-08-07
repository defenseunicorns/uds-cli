// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportedBlobOperations(t *testing.T) {
	blobDir := t.TempDir()
	data := []byte("blob bytes")
	wantDigest := digest.FromBytes(data)

	parsed, err := udsoci.ParseDigest(wantDigest.String())
	require.NoError(t, err)
	assert.Equal(t, wantDigest, parsed)
	assert.Equal(t, wantDigest, udsoci.SHA256Digest(wantDigest.Encoded()))

	require.NoError(t, udsoci.WriteBlobBytesIfMissingAndVerify(blobDir, wantDigest, data))
	assertBlobContent(t, blobDir, wantDigest, data)

	readerData := []byte("reader blob")
	readerDigest := digest.FromBytes(readerData)
	require.NoError(t, udsoci.WriteBlobReaderIfMissingAndVerify(blobDir, readerDigest, bytes.NewReader(readerData)))
	assertBlobContent(t, blobDir, readerDigest, readerData)

	sourcePath := filepath.Join(t.TempDir(), "source.bin")
	copyData := []byte("copied blob")
	copyDigest := digest.FromBytes(copyData)
	require.NoError(t, os.WriteFile(sourcePath, copyData, 0o600))
	require.NoError(t, udsoci.CopyBlobFileIfMissingAndVerify(blobDir, sourcePath, copyDigest))
	assertBlobContent(t, blobDir, copyDigest, copyData)
}

func TestCopyRequiredBlobsFromLayoutExport(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	configData := []byte(`{"architecture":"amd64"}`)
	layerData := []byte("layer data")
	configDigest := digest.FromBytes(configData)
	layerDigest := digest.FromBytes(layerData)

	manifestData, err := json.Marshal(udsoci.OciImageManifest{
		SchemaVersion: 2,
		Config: udsoci.OciDescriptor{
			Digest: configDigest.String(), Size: int64(len(configData)),
		},
		Layers: []udsoci.OciDescriptor{{
			Digest: layerDigest.String(), Size: int64(len(layerData)),
		}},
	})
	require.NoError(t, err)
	manifestDigest := digest.FromBytes(manifestData)

	for d, data := range map[digest.Digest][]byte{
		configDigest:   configData,
		layerDigest:    layerData,
		manifestDigest: manifestData,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, d.Encoded()), data, 0o600))
	}

	require.NoError(t, udsoci.CopyRequiredBlobsFromLayout(dstDir, srcDir, []udsoci.OciManifest{{
		Digest: manifestDigest.String(), Size: int64(len(manifestData)),
	}}))

	assertBlobContent(t, dstDir, configDigest, configData)
	assertBlobContent(t, dstDir, layerDigest, layerData)
	assertBlobContent(t, dstDir, manifestDigest, manifestData)
}

func assertBlobContent(t *testing.T, blobDir string, d digest.Digest, want []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(blobDir, d.Encoded()))
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
