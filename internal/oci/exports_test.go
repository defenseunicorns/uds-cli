// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci_test

import (
	"bytes"
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

func assertBlobContent(t *testing.T, blobDir string, d digest.Digest, want []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(blobDir, d.Encoded()))
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
