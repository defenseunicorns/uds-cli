// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindOCILayoutRoot(t *testing.T) {
	t.Parallel()
	t.Run("finds OCI layout in root directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "blobs", "sha256"), tempDirPerm))
		require.NoError(t, os.WriteFile(filepath.Join(root, "oci-layout"), []byte(`{"imageLayoutVersion": "1.0.0"}`), tmpFilePerm))
		require.NoError(t, os.WriteFile(filepath.Join(root, "index.json"), []byte(`{"schemaVersion": 2}`), tmpFilePerm))

		found, err := findOCILayoutRoot(root)
		require.NoError(t, err)
		assert.Equal(t, root, found)
	})

	t.Run("finds OCI layout in oci subdirectory", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
		root := t.TempDir()
		// Create some random files but no OCI layout
		require.NoError(t, os.WriteFile(filepath.Join(root, "random.txt"), []byte("not an oci layout"), tmpFilePerm))

		found, err := findOCILayoutRoot(root)
		require.Error(t, err)
		require.ErrorContains(t, err, "no OCI image layout found")
		assert.Empty(t, found)
	})

	t.Run("prefers root over subdirectories", func(t *testing.T) {
		t.Parallel()
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

func TestIsOCILayoutDir(t *testing.T) {
	t.Parallel()
	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "blobs", "sha256"), tempDirPerm))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "oci-layout"), []byte(`{"imageLayoutVersion": "1.0.0"}`), tmpFilePerm))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{"schemaVersion": 2}`), tmpFilePerm))
		assert.True(t, isOCILayoutDir(dir))
	})

	t.Run("missing oci-layout", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "blobs", "sha256"), tempDirPerm))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{"schemaVersion": 2}`), tmpFilePerm))
		assert.False(t, isOCILayoutDir(dir))
	})
}

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

type verifiableLayout struct {
	ManifestHex string
	ConfigHex   string
	LayerHex    string
}

// buildVerifiableOCILayout creates a minimal but fully valid OCI layout in ociDir
// with one manifest containing a config blob and one layer blob. All digests and
// sizes are correct. Returns the hex digests for targeted corruption in tests.
func buildVerifiableOCILayout(t *testing.T, ociDir string) verifiableLayout {
	t.Helper()

	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

	// Write layer blob.
	layerData := []byte("layer content for verification test")
	layerHex := writeTestBlob(t, blobDir, layerData)

	// Write config blob.
	configData := []byte("{}")
	configHex := writeTestBlob(t, blobDir, configData)

	// Build and write manifest blob.
	manifest := ociImageManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: ociDescriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    "sha256:" + configHex,
			Size:      int64(len(configData)),
		},
		Layers: []ociDescriptor{{
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
			Digest:    "sha256:" + layerHex,
			Size:      int64(len(layerData)),
		}},
	}
	manifestBytes, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestHex := writeTestBlob(t, blobDir, manifestBytes)

	// Write index.json.
	idx := ociIndex{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.index.v1+json",
		Manifests: []ociManifest{{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:" + manifestHex,
			Size:      int64(len(manifestBytes)),
		}},
	}
	idxBytes, err := json.Marshal(idx)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), idxBytes, tmpFilePerm))

	return verifiableLayout{
		ManifestHex: manifestHex,
		ConfigHex:   configHex,
		LayerHex:    layerHex,
	}
}

// rewriteIndexMediaType reads index.json, sets the first manifest entry's
// MediaType to the given value, and writes it back.
func rewriteIndexMediaType(t *testing.T, ociDir, mediaType string) {
	t.Helper()
	idxPath := filepath.Join(ociDir, "index.json")
	data, err := os.ReadFile(idxPath)
	require.NoError(t, err)
	var idx ociIndex
	require.NoError(t, json.Unmarshal(data, &idx))
	require.NotEmpty(t, idx.Manifests)
	idx.Manifests[0].MediaType = mediaType
	out, err := json.Marshal(idx)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(idxPath, out, tmpFilePerm))
}

// corruptBlob alters a blob file where its content no longer matches its digest
// without changing the file size so size checks pass but digest checks fail.
func corruptBlob(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEmpty(t, data, "cannot corrupt empty blob")
	data[len(data)-1] ^= 0xFF
	require.NoError(t, os.WriteFile(path, data, tmpFilePerm))
}

func TestVerifyOCILayoutDigests(t *testing.T) {
	t.Parallel()

	t.Run("valid layout passes verification", func(t *testing.T) {
		t.Parallel()
		ociDir := t.TempDir()
		buildVerifiableOCILayout(t, ociDir)

		err := verifyOCILayoutDigests(t.Context(), iostreams.IOStreams{}, ociDir)
		assert.NoError(t, err)
	})

	// Table-driven: build valid layout, mutate one thing, check error.
	mutationTests := []struct {
		name    string
		mutate  func(t *testing.T, ociDir string, layout verifiableLayout)
		wantErr string
	}{
		{
			name: "corrupted manifest blob",
			mutate: func(t *testing.T, ociDir string, layout verifiableLayout) {
				corruptBlob(t, filepath.Join(ociDir, "blobs", "sha256", layout.ManifestHex))
			},
			wantErr: "digest mismatch",
		},
		{
			name: "corrupted config blob",
			mutate: func(t *testing.T, ociDir string, layout verifiableLayout) {
				corruptBlob(t, filepath.Join(ociDir, "blobs", "sha256", layout.ConfigHex))
			},
			wantErr: "digest mismatch",
		},
		{
			name: "corrupted layer blob",
			mutate: func(t *testing.T, ociDir string, layout verifiableLayout) {
				corruptBlob(t, filepath.Join(ociDir, "blobs", "sha256", layout.LayerHex))
			},
			wantErr: "digest mismatch",
		},
		{
			name: "missing layer blob",
			mutate: func(t *testing.T, ociDir string, layout verifiableLayout) {
				require.NoError(t, os.Remove(filepath.Join(ociDir, "blobs", "sha256", layout.LayerHex)))
			},
			wantErr: "opening blob",
		},
		{
			name: "missing manifest blob",
			mutate: func(t *testing.T, ociDir string, layout verifiableLayout) {
				require.NoError(t, os.Remove(filepath.Join(ociDir, "blobs", "sha256", layout.ManifestHex)))
			},
			wantErr: "opening blob",
		},
		{
			name: "size mismatch on manifest blob",
			mutate: func(t *testing.T, ociDir string, layout verifiableLayout) {
				require.NoError(t, os.WriteFile(
					filepath.Join(ociDir, "blobs", "sha256", layout.ManifestHex),
					[]byte("short"), tmpFilePerm))
			},
			wantErr: "size mismatch",
		},
		{
			name: "unknown media type in index",
			mutate: func(t *testing.T, ociDir string, layout verifiableLayout) {
				rewriteIndexMediaType(t, ociDir, "application/vnd.unknown.type")
			},
			wantErr: "unsupported manifest media type",
		},
		{
			name: "empty media type in index",
			mutate: func(t *testing.T, ociDir string, layout verifiableLayout) {
				rewriteIndexMediaType(t, ociDir, "")
			},
			wantErr: "unsupported manifest media type",
		},
	}

	for _, tt := range mutationTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ociDir := t.TempDir()
			layout := buildVerifiableOCILayout(t, ociDir)

			tt.mutate(t, ociDir, layout)

			err := verifyOCILayoutDigests(t.Context(), iostreams.IOStreams{}, ociDir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}

	t.Run("missing index.json", func(t *testing.T) {
		t.Parallel()
		ociDir := t.TempDir()

		err := verifyOCILayoutDigests(t.Context(), iostreams.IOStreams{}, ociDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index.json")
	})

	t.Run("size mismatch on layer blob", func(t *testing.T) {
		t.Parallel()
		ociDir := t.TempDir()
		blobDir := filepath.Join(ociDir, "blobs", "sha256")
		require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

		layerData := []byte("some layer data")
		layerHex := writeTestBlob(t, blobDir, layerData)

		configData := []byte("{}")
		configHex := writeTestBlob(t, blobDir, configData)

		manifest := ociImageManifest{
			SchemaVersion: 2,
			Config: ociDescriptor{
				Digest: "sha256:" + configHex,
				Size:   int64(len(configData)),
			},
			Layers: []ociDescriptor{{
				Digest: "sha256:" + layerHex,
				Size:   int64(len(layerData)) + 100, // wrong size
			}},
		}
		manifestBytes, err := json.Marshal(manifest)
		require.NoError(t, err)
		manifestHex := writeTestBlob(t, blobDir, manifestBytes)

		idx := ociIndex{
			SchemaVersion: 2,
			Manifests: []ociManifest{{
				MediaType: ocispec.MediaTypeImageManifest,
				Digest:    "sha256:" + manifestHex,
				Size:      int64(len(manifestBytes)),
			}},
		}
		idxBytes, err := json.Marshal(idx)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), idxBytes, tmpFilePerm))

		err = verifyOCILayoutDigests(t.Context(), iostreams.IOStreams{}, ociDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "size mismatch")
	})

	t.Run("multiple manifests all verified", func(t *testing.T) {
		t.Parallel()
		ociDir := t.TempDir()
		blobDir := filepath.Join(ociDir, "blobs", "sha256")
		require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

		var manifests []ociManifest
		for i := range 2 {
			layerData := []byte("layer " + string(rune('A'+i)))
			layerHex := writeTestBlob(t, blobDir, layerData)

			configData := []byte("{}")
			configHex := writeTestBlob(t, blobDir, configData)

			im := ociImageManifest{
				SchemaVersion: 2,
				Config: ociDescriptor{
					Digest: "sha256:" + configHex,
					Size:   int64(len(configData)),
				},
				Layers: []ociDescriptor{{
					Digest: "sha256:" + layerHex,
					Size:   int64(len(layerData)),
				}},
			}
			mBytes, err := json.Marshal(im)
			require.NoError(t, err)
			mHex := writeTestBlob(t, blobDir, mBytes)

			manifests = append(manifests, ociManifest{
				MediaType: ocispec.MediaTypeImageManifest,
				Digest:    "sha256:" + mHex,
				Size:      int64(len(mBytes)),
			})
		}

		idx := ociIndex{SchemaVersion: 2, Manifests: manifests}
		idxBytes, err := json.Marshal(idx)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), idxBytes, tmpFilePerm))

		err = verifyOCILayoutDigests(t.Context(), iostreams.IOStreams{}, ociDir)
		assert.NoError(t, err)
	})

	t.Run("manifest with empty config digest is skipped", func(t *testing.T) {
		t.Parallel()
		ociDir := t.TempDir()
		blobDir := filepath.Join(ociDir, "blobs", "sha256")
		require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

		layerData := []byte("layer data")
		layerHex := writeTestBlob(t, blobDir, layerData)

		im := ociImageManifest{
			SchemaVersion: 2,
			Config:        ociDescriptor{},
			Layers: []ociDescriptor{{
				Digest: "sha256:" + layerHex,
				Size:   int64(len(layerData)),
			}},
		}
		mBytes, err := json.Marshal(im)
		require.NoError(t, err)
		mHex := writeTestBlob(t, blobDir, mBytes)

		idx := ociIndex{
			SchemaVersion: 2,
			Manifests: []ociManifest{{
				MediaType: ocispec.MediaTypeImageManifest,
				Digest:    "sha256:" + mHex,
				Size:      int64(len(mBytes)),
			}},
		}
		idxBytes, err := json.Marshal(idx)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), idxBytes, tmpFilePerm))

		err = verifyOCILayoutDigests(t.Context(), iostreams.IOStreams{}, ociDir)
		assert.NoError(t, err)
	})

	t.Run("nested index recurses into child manifests", func(t *testing.T) {
		t.Parallel()
		ociDir := t.TempDir()
		blobDir := filepath.Join(ociDir, "blobs", "sha256")
		require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

		// Build a child image manifest with config + layer.
		layerData := []byte("nested layer")
		layerHex := writeTestBlob(t, blobDir, layerData)

		configData := []byte("{}")
		configHex := writeTestBlob(t, blobDir, configData)

		childManifest := ociImageManifest{
			SchemaVersion: 2,
			Config: ociDescriptor{
				Digest: "sha256:" + configHex,
				Size:   int64(len(configData)),
			},
			Layers: []ociDescriptor{{
				Digest: "sha256:" + layerHex,
				Size:   int64(len(layerData)),
			}},
		}
		childBytes, err := json.Marshal(childManifest)
		require.NoError(t, err)
		childHex := writeTestBlob(t, blobDir, childBytes)

		// Build a nested index that references the child manifest.
		nestedIdx := ociIndex{
			SchemaVersion: 2,
			Manifests: []ociManifest{{
				MediaType: ocispec.MediaTypeImageManifest,
				Digest:    "sha256:" + childHex,
				Size:      int64(len(childBytes)),
			}},
		}
		nestedIdxBytes, err := json.Marshal(nestedIdx)
		require.NoError(t, err)
		nestedIdxHex := writeTestBlob(t, blobDir, nestedIdxBytes)

		// Top-level index points to the nested index.
		topIdx := ociIndex{
			SchemaVersion: 2,
			Manifests: []ociManifest{{
				MediaType: ocispec.MediaTypeImageIndex,
				Digest:    "sha256:" + nestedIdxHex,
				Size:      int64(len(nestedIdxBytes)),
			}},
		}
		topIdxBytes, err := json.Marshal(topIdx)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), topIdxBytes, tmpFilePerm))

		err = verifyOCILayoutDigests(t.Context(), iostreams.IOStreams{}, ociDir)
		assert.NoError(t, err)
	})

	t.Run("nested index detects corrupted child blob", func(t *testing.T) {
		t.Parallel()
		ociDir := t.TempDir()
		blobDir := filepath.Join(ociDir, "blobs", "sha256")
		require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

		layerData := []byte("nested layer")
		layerHex := writeTestBlob(t, blobDir, layerData)

		configData := []byte("{}")
		configHex := writeTestBlob(t, blobDir, configData)

		childManifest := ociImageManifest{
			SchemaVersion: 2,
			Config: ociDescriptor{
				Digest: "sha256:" + configHex,
				Size:   int64(len(configData)),
			},
			Layers: []ociDescriptor{{
				Digest: "sha256:" + layerHex,
				Size:   int64(len(layerData)),
			}},
		}
		childBytes, err := json.Marshal(childManifest)
		require.NoError(t, err)
		childHex := writeTestBlob(t, blobDir, childBytes)

		nestedIdx := ociIndex{
			SchemaVersion: 2,
			Manifests: []ociManifest{{
				MediaType: ocispec.MediaTypeImageManifest,
				Digest:    "sha256:" + childHex,
				Size:      int64(len(childBytes)),
			}},
		}
		nestedIdxBytes, err := json.Marshal(nestedIdx)
		require.NoError(t, err)
		nestedIdxHex := writeTestBlob(t, blobDir, nestedIdxBytes)

		topIdx := ociIndex{
			SchemaVersion: 2,
			Manifests: []ociManifest{{
				MediaType: ocispec.MediaTypeImageIndex,
				Digest:    "sha256:" + nestedIdxHex,
				Size:      int64(len(nestedIdxBytes)),
			}},
		}
		topIdxBytes, err := json.Marshal(topIdx)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), topIdxBytes, tmpFilePerm))

		// Corrupt the layer blob inside the nested index.
		corruptBlob(t, filepath.Join(blobDir, layerHex))

		err = verifyOCILayoutDigests(t.Context(), iostreams.IOStreams{}, ociDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nested index")
		assert.Contains(t, err.Error(), "digest mismatch")
	})

	t.Run("zero declared size with non-empty blob fails", func(t *testing.T) {
		t.Parallel()
		ociDir := t.TempDir()
		blobDir := filepath.Join(ociDir, "blobs", "sha256")
		require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

		layerData := []byte("non-empty layer")
		layerHex := writeTestBlob(t, blobDir, layerData)

		configData := []byte("{}")
		configHex := writeTestBlob(t, blobDir, configData)

		// Declare layer size as 0 even though blob has content.
		im := ociImageManifest{
			SchemaVersion: 2,
			Config: ociDescriptor{
				Digest: "sha256:" + configHex,
				Size:   int64(len(configData)),
			},
			Layers: []ociDescriptor{{
				Digest: "sha256:" + layerHex,
				Size:   0,
			}},
		}
		mBytes, err := json.Marshal(im)
		require.NoError(t, err)
		mHex := writeTestBlob(t, blobDir, mBytes)

		idx := ociIndex{
			SchemaVersion: 2,
			Manifests: []ociManifest{{
				MediaType: ocispec.MediaTypeImageManifest,
				Digest:    "sha256:" + mHex,
				Size:      int64(len(mBytes)),
			}},
		}
		idxBytes, err := json.Marshal(idx)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), idxBytes, tmpFilePerm))

		err = verifyOCILayoutDigests(t.Context(), iostreams.IOStreams{}, ociDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "size mismatch")
	})
}
