// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
)

func TestIsBundleIndex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		idx  ociIndex
		want bool
	}{
		{
			name: "index declaring the bundle artifactType is a bundle",
			idx:  ociIndex{SchemaVersion: 2, ArtifactType: MediaTypeBundle},
			want: true,
		},
		{
			name: "index without an artifactType is not a bundle",
			idx: ociIndex{SchemaVersion: 2, Manifests: []ociManifest{
				{Digest: "sha256:aaa", ArtifactType: MediaTypeBundleDefinition},
			}},
			want: false,
		},
		{
			name: "index with a different artifactType is not a bundle",
			idx:  ociIndex{SchemaVersion: 2, ArtifactType: "application/vnd.example.other.v1"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isBundleIndex(tt.idx))
		})
	}
}

func TestSortManifestsByDigest(t *testing.T) {
	t.Parallel()
	manifests := []ociManifest{
		{Digest: "sha256:ccc"},
		{Digest: "sha256:aaa"},
		{Digest: "sha256:bbb"},
	}
	sortManifestsByDigest(manifests)
	assert.Equal(t, "sha256:aaa", manifests[0].Digest)
	assert.Equal(t, "sha256:bbb", manifests[1].Digest)
	assert.Equal(t, "sha256:ccc", manifests[2].Digest)
}

func TestFindBundleDefinitionEntry_Found(t *testing.T) {
	t.Parallel()
	idx := ociIndex{
		SchemaVersion: 2,
		Manifests: []ociManifest{
			{Digest: "sha256:aaa", ArtifactType: "other"},
			{Digest: "sha256:bbb", ArtifactType: MediaTypeBundleDefinition},
		},
	}

	entry, pos, err := findBundleDefinitionEntry(idx)
	require.NoError(t, err)
	assert.Equal(t, 1, pos)
	assert.Equal(t, "sha256:bbb", entry.Digest)
}

func TestFindBundleDefinitionEntry_NotFound(t *testing.T) {
	t.Parallel()
	idx := ociIndex{
		SchemaVersion: 2,
		Manifests: []ociManifest{
			{Digest: "sha256:aaa", ArtifactType: "other"},
		},
	}

	_, _, err := findBundleDefinitionEntry(idx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// pushIndexToMemory marshals idx, pushes it into a fresh in-memory store, and
// returns the store, descriptor, and raw bytes. The descriptor is also tagged
// under tag when non-empty.
func pushIndexToMemory(t *testing.T, idx ociIndex, tag string) (*memory.Store, ocispec.Descriptor, []byte) {
	t.Helper()
	store := memory.New()
	data, err := json.Marshal(idx)
	require.NoError(t, err)
	desc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, data)
	require.NoError(t, store.Push(t.Context(), desc, bytes.NewReader(data)))
	if tag != "" {
		require.NoError(t, store.Tag(t.Context(), desc, tag))
	}
	return store, desc, data
}

func TestResolveBundleChild(t *testing.T) {
	t.Parallel()

	childIdx := ociIndex{
		SchemaVersion: 2,
		MediaType:     ocispec.MediaTypeImageIndex,
		ArtifactType:  MediaTypeBundle,
		Manifests:     []ociManifest{},
		Annotations:   map[string]string{AnnotationBundleArchitecture: "amd64"},
	}

	t.Run("returns a directly-addressed child as-is", func(t *testing.T) {
		t.Parallel()
		store, desc, data := pushIndexToMemory(t, childIdx, "v1")

		gotDesc, gotData, err := resolveBundleChild(t.Context(), store, "v1", "amd64")
		require.NoError(t, err)
		assert.Equal(t, desc.Digest, gotDesc.Digest)
		assert.Equal(t, data, gotData)
	})

	t.Run("selects the matching architecture from a root index", func(t *testing.T) {
		t.Parallel()
		store, childDesc, childData := pushIndexToMemory(t, childIdx, "")

		root := ociIndex{
			SchemaVersion: 2,
			MediaType:     ocispec.MediaTypeImageIndex,
			Manifests: []ociManifest{{
				MediaType:    ocispec.MediaTypeImageIndex,
				ArtifactType: MediaTypeBundle,
				Digest:       childDesc.Digest.String(),
				Size:         childDesc.Size,
				Platform:     &ocispec.Platform{Architecture: "amd64", OS: "multi"},
			}},
		}
		rootData, err := json.Marshal(root)
		require.NoError(t, err)
		rootDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, rootData)
		require.NoError(t, store.Push(t.Context(), rootDesc, bytes.NewReader(rootData)))
		require.NoError(t, store.Tag(t.Context(), rootDesc, "v1"))

		gotDesc, gotData, err := resolveBundleChild(t.Context(), store, "v1", "amd64")
		require.NoError(t, err)
		assert.Equal(t, childDesc.Digest, gotDesc.Digest)
		assert.Equal(t, childData, gotData)

		_, _, err = resolveBundleChild(t.Context(), store, "v1", "arm64")
		require.ErrorContains(t, err, `no bundle for architecture "arm64"`)
	})

	t.Run("rejects an index that is neither child nor root", func(t *testing.T) {
		t.Parallel()
		store, _, _ := pushIndexToMemory(t, ociIndex{
			SchemaVersion: 2,
			MediaType:     ocispec.MediaTypeImageIndex,
			Manifests: []ociManifest{{
				MediaType: ocispec.MediaTypeImageManifest,
				Digest:    "sha256:1111111111111111111111111111111111111111111111111111111111111111",
				Size:      2,
			}},
		}, "v1")

		_, _, err := resolveBundleChild(t.Context(), store, "v1", "amd64")
		require.ErrorContains(t, err, "does not appear to be a UDS bundle")
	})

	t.Run("rejects non-index content", func(t *testing.T) {
		t.Parallel()
		store := memory.New()
		data := []byte("not json")
		// A non-manifest media type so the memory store does not parse successors.
		desc := content.NewDescriptorFromBytes("application/octet-stream", data)
		require.NoError(t, store.Push(t.Context(), desc, bytes.NewReader(data)))
		require.NoError(t, store.Tag(t.Context(), desc, "v1"))

		_, _, err := resolveBundleChild(t.Context(), store, "v1", "amd64")
		require.ErrorContains(t, err, "content is not an OCI index")
	})

	t.Run("errors when the reference does not resolve", func(t *testing.T) {
		t.Parallel()
		_, _, err := resolveBundleChild(t.Context(), memory.New(), "missing", "amd64")
		require.ErrorContains(t, err, "resolving missing")
	})
}

// flakyResolveTarget wraps an oras.Target and fails Resolve with a transient
// (non-NotFound) error, simulating a registry hiccup while reading the
// existing root index.
type flakyResolveTarget struct {
	oras.Target
}

// Resolve returns a transient error for root-index merge tests.
func (f *flakyResolveTarget) Resolve(context.Context, string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{}, fmt.Errorf("registry unavailable")
}

func TestMergeRootIndex(t *testing.T) {
	t.Parallel()

	child := ocispec.Descriptor{
		MediaType:    ocispec.MediaTypeImageIndex,
		ArtifactType: MediaTypeBundle,
		Digest:       godigest.FromString("child amd64"),
		Size:         42,
		Platform:     &ocispec.Platform{Architecture: "amd64", OS: "multi"},
	}

	t.Run("missing tag publishes a fresh root", func(t *testing.T) {
		t.Parallel()
		rootBytes, _, err := mergeRootIndex(t.Context(), memory.New(), "v1", child)
		require.NoError(t, err)

		var root ocispec.Index
		require.NoError(t, json.Unmarshal(rootBytes, &root))
		require.Len(t, root.Manifests, 1)
		assert.Equal(t, child.Digest, root.Manifests[0].Digest)
	})

	t.Run("errors when the existing root cannot be read instead of clobbering it", func(t *testing.T) {
		t.Parallel()
		_, _, err := mergeRootIndex(t.Context(), &flakyResolveTarget{memory.New()}, "v1", child)
		require.ErrorContains(t, err, "reading existing root index")
		require.ErrorContains(t, err, "registry unavailable")
	})
}
