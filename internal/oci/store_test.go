// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	godigest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorePruneUnreferencedBlobsPreservesReachableAndRemovesOrphans(t *testing.T) {
	store, err := CreateStore(t.TempDir())
	require.NoError(t, err)
	root, config, layer := pushTestManifestGraph(t, store, []byte("layer"))
	orphan, err := store.PushBytes(t.Context(), "application/vnd.test.orphan", []byte("orphan"))
	require.NoError(t, err)

	require.NoError(t, store.PruneUnreferencedBlobs(t.Context(), iostreams.IOStreams{}, []ocispec.Descriptor{root}))

	assertBlobExists(t, store, root)
	assertBlobExists(t, store, config)
	assertBlobExists(t, store, layer)
	assertBlobMissing(t, store, orphan)
}

func TestStoreVerifyGraph(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*testing.T, *Store, ocispec.Descriptor, ocispec.Descriptor) ocispec.Descriptor
		wantError string
	}{
		{
			name: "valid graph with large leaf",
			mutate: func(t *testing.T, store *Store, root, _ ocispec.Descriptor) ocispec.Descriptor {
				t.Helper()
				return root
			},
		},
		{
			name: "missing leaf blob",
			mutate: func(t *testing.T, store *Store, root, layer ocispec.Descriptor) ocispec.Descriptor {
				t.Helper()
				path, err := store.BlobPath(layer.Digest)
				require.NoError(t, err)
				require.NoError(t, os.Remove(path))
				return root
			},
			wantError: "verifying " + godigest.FromBytes(largeTestLeaf()).String(),
		},
		{
			name: "digest mismatch",
			mutate: func(t *testing.T, store *Store, root, layer ocispec.Descriptor) ocispec.Descriptor {
				t.Helper()
				path, err := store.BlobPath(layer.Digest)
				require.NoError(t, err)
				require.NoError(t, os.Chmod(path, tmpFilePerm))
				require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("z"), int(layer.Size)), tmpFilePerm))
				return root
			},
			wantError: "mismatched digest",
		},
		{
			name: "size mismatch",
			mutate: func(t *testing.T, store *Store, _, layer ocispec.Descriptor) ocispec.Descriptor {
				t.Helper()
				layer.Size++
				return pushManifestForLayer(t, store, layer)
			},
			wantError: "unexpected EOF",
		},
		{
			name: "conflicting sizes",
			mutate: func(t *testing.T, store *Store, root, _ ocispec.Descriptor) ocispec.Descriptor {
				t.Helper()
				second := root
				second.Size++
				indexBytes, err := json.Marshal(ocispec.Index{
					Versioned: specs.Versioned{SchemaVersion: 2},
					MediaType: ocispec.MediaTypeImageIndex,
					Manifests: []ocispec.Descriptor{root, second},
				})
				require.NoError(t, err)
				index := NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, indexBytes)
				require.NoError(t, PushDescriptorBytes(t.Context(), store, index, indexBytes))
				return index
			},
			wantError: "conflicting sizes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := CreateStore(t.TempDir())
			require.NoError(t, err)
			root, _, layer := pushTestManifestGraph(t, store, largeTestLeaf())

			err = store.VerifyGraph(t.Context(), []ocispec.Descriptor{tt.mutate(t, store, root, layer)})

			if tt.wantError == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantError)
		})
	}
}

func pushTestManifestGraph(t *testing.T, store *Store, layerBytes []byte) (ocispec.Descriptor, ocispec.Descriptor, ocispec.Descriptor) {
	t.Helper()
	config, err := store.PushBytes(t.Context(), "application/vnd.test.config", []byte("{}"))
	require.NoError(t, err)
	layer, err := store.PushBytes(t.Context(), "application/vnd.test.layer", layerBytes)
	require.NoError(t, err)
	return pushManifestWithConfigAndLayer(t, store, config, layer), config, layer
}

func pushManifestForLayer(t *testing.T, store *Store, layer ocispec.Descriptor) ocispec.Descriptor {
	t.Helper()
	config, err := store.PushBytes(t.Context(), "application/vnd.test.config", []byte("{}"))
	require.NoError(t, err)
	return pushManifestWithConfigAndLayer(t, store, config, layer)
}

func pushManifestWithConfigAndLayer(t *testing.T, store *Store, config, layer ocispec.Descriptor) ocispec.Descriptor {
	t.Helper()
	manifestBytes, err := json.Marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    config,
		Layers:    []ocispec.Descriptor{layer},
	})
	require.NoError(t, err)
	root := NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestBytes)
	require.NoError(t, PushDescriptorBytes(t.Context(), store, root, manifestBytes))
	return root
}

func largeTestLeaf() []byte {
	return bytes.Repeat([]byte("x"), 2*1024*1024)
}

func assertBlobExists(t *testing.T, store *Store, desc ocispec.Descriptor) {
	t.Helper()
	path, err := store.BlobPath(desc.Digest)
	require.NoError(t, err)
	assert.FileExists(t, path)
}

func assertBlobMissing(t *testing.T, store *Store, desc ocispec.Descriptor) {
	t.Helper()
	path, err := store.BlobPath(desc.Digest)
	require.NoError(t, err)
	_, err = os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}
