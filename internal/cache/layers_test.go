// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cache

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteLayerPublishesVerifiedContentFromLayersDirectory(t *testing.T) {
	cacheDir := t.TempDir()
	content := []byte("image layer contents")
	desc := ocispec.Descriptor{Digest: godigest.FromBytes(content), Size: int64(len(content))}
	layersDir := filepath.Join(cacheDir, LayersDirName)
	finalPath := filepath.Join(layersDir, desc.Digest.Encoded())
	src := &observingReader{
		Reader: bytes.NewReader(content),
		onRead: func() {
			entries, err := os.ReadDir(layersDir)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Regexp(t, `^\.uds-layer-`, entries[0].Name())
			_, err = os.Stat(finalPath)
			assert.ErrorIs(t, err, os.ErrNotExist)
		},
	}

	require.NoError(t, WriteLayer(cacheDir, desc, src))
	actual, err := os.ReadFile(finalPath)
	require.NoError(t, err)
	assert.Equal(t, content, actual)
	entries, err := os.ReadDir(layersDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "temporary file should be removed after publication")
	assert.Equal(t, desc.Digest.Encoded(), entries[0].Name())
}

func TestWriteLayerRejectsInvalidContentWithoutReplacingExistingLayer(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content []byte
	}{
		{name: "wrong digest", content: []byte("BADid image layer")},
		{name: "wrong size", content: []byte("invalid image layer")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cacheDir := t.TempDir()
			content := []byte("valid image layer")
			desc := ocispec.Descriptor{Digest: godigest.FromBytes(content), Size: int64(len(content))}
			require.NoError(t, WriteLayer(cacheDir, desc, bytes.NewReader(content)))
			finalPath := filepath.Join(cacheDir, LayersDirName, desc.Digest.Encoded())

			err := WriteLayer(cacheDir, desc, bytes.NewReader(tc.content))
			require.Error(t, err)
			actual, err := os.ReadFile(finalPath)
			require.NoError(t, err)
			assert.Equal(t, content, actual)
			entries, err := os.ReadDir(filepath.Join(cacheDir, LayersDirName))
			require.NoError(t, err)
			require.Len(t, entries, 1, "failed write should leave no temporary file")
		})
	}
}

func TestWriteLayerRejectsInvalidDescriptorBeforeCreatingCache(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	err := WriteLayer(cacheDir, ocispec.Descriptor{}, bytes.NewReader(nil))
	require.Error(t, err)
	_, err = os.Stat(cacheDir)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

type observingReader struct {
	io.Reader
	onRead func()
}

func (r *observingReader) Read(p []byte) (int, error) {
	r.onRead()
	r.onRead = func() {}
	return r.Reader.Read(p)
}
