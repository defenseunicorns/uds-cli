// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package boci

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
)

// manifestFor builds an image manifest descriptor annotated with the given base image name.
func manifestFor(imgName string) ocispec.Descriptor {
	return ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString(imgName),
		Annotations: map[string]string{
			ocispec.AnnotationBaseImageName: imgName,
		},
	}
}

// TestFilterImageIndex ensures that images referenced only via imageArchives are retained when
// filtering the image index. Regression test for CLI-235: deploying an OCI bundle whose Zarf
// package declares an imageArchive previously failed with "checksum missing in layout" because
// FilterImageIndex only inspected component.Images and dropped the archive's manifest/blobs.
func TestFilterImageIndex(t *testing.T) {
	const (
		archiveImg = "ghcr.io/uds-packages/tinkerbell/hookos-artifacts:0.1.0"
		regularImg = "ghcr.io/defenseunicorns/some-image:1.0.0"
		unusedImg  = "ghcr.io/defenseunicorns/not-referenced:2.0.0"
	)

	index := ocispec.Index{
		Manifests: []ocispec.Descriptor{
			manifestFor(archiveImg),
			manifestFor(regularImg),
			manifestFor(unusedImg),
		},
	}

	tests := []struct {
		name       string
		components []v1alpha1.ZarfComponent
		wantImages []string
	}{
		{
			name: "only imageArchives",
			components: []v1alpha1.ZarfComponent{
				{
					Name: "hookos-artifacts",
					ImageArchives: []v1alpha1.ImageArchive{
						{Path: "image.tar", Images: []string{archiveImg}},
					},
				},
			},
			wantImages: []string{archiveImg},
		},
		{
			name: "only images",
			components: []v1alpha1.ZarfComponent{
				{Name: "regular", Images: []string{regularImg}},
			},
			wantImages: []string{regularImg},
		},
		{
			name: "both images and imageArchives",
			components: []v1alpha1.ZarfComponent{
				{
					Name:   "mixed",
					Images: []string{regularImg},
					ImageArchives: []v1alpha1.ImageArchive{
						{Path: "image.tar", Images: []string{archiveImg}},
					},
				},
			},
			wantImages: []string{regularImg, archiveImg},
		},
		{
			name: "no matching images",
			components: []v1alpha1.ZarfComponent{
				{Name: "empty"},
			},
			wantImages: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifests, err := FilterImageIndex(tt.components, index)
			require.NoError(t, err)

			gotDigests := make([]string, 0, len(manifests))
			for _, m := range manifests {
				gotDigests = append(gotDigests, m.Digest.String())
			}

			wantDigests := make([]string, 0, len(tt.wantImages))
			for _, img := range tt.wantImages {
				wantDigests = append(wantDigests, digest.FromString(img).String())
			}

			require.ElementsMatch(t, wantDigests, gotDigests)
		})
	}
}

func TestCollectImageDescriptors(t *testing.T) {
	ctx := context.Background()
	store := memory.New()

	configDesc := pushTestBlob(t, ctx, store, ocispec.MediaTypeImageConfig, []byte(`{"architecture":"amd64","os":"linux"}`))
	layerDesc := pushTestBlob(t, ctx, store, ocispec.MediaTypeImageLayer, []byte("shared layer"))

	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{layerDesc},
		Annotations: map[string]string{
			"test.architecture": "amd64",
		},
	}
	amdManifestDesc := pushTestJSON(t, ctx, store, ocispec.MediaTypeImageManifest, manifest)
	manifest.Annotations["test.architecture"] = "arm64"
	armManifestDesc := pushTestJSON(t, ctx, store, ocispec.MediaTypeImageManifest, manifest)

	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{
			amdManifestDesc,
			armManifestDesc,
		},
	}
	indexDesc := pushTestJSON(t, ctx, store, ocispec.MediaTypeImageIndex, index)

	t.Run("image manifest", func(t *testing.T) {
		descriptors, err := collectImageDescriptors(ctx, store, amdManifestDesc, map[string]struct{}{})
		require.NoError(t, err)
		require.ElementsMatch(t,
			[]digest.Digest{amdManifestDesc.Digest, configDesc.Digest, layerDesc.Digest},
			descriptorDigests(descriptors),
		)
	})

	t.Run("multi-platform image index", func(t *testing.T) {
		descriptors, err := collectImageDescriptors(ctx, store, indexDesc, map[string]struct{}{})
		require.NoError(t, err)
		require.ElementsMatch(t,
			[]digest.Digest{
				indexDesc.Digest,
				amdManifestDesc.Digest,
				armManifestDesc.Digest,
				configDesc.Digest,
				layerDesc.Digest,
			},
			descriptorDigests(descriptors),
		)
	})

	t.Run("empty digest", func(t *testing.T) {
		_, err := collectImageDescriptors(ctx, store, ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest}, map[string]struct{}{})
		require.EqualError(t, err, `image descriptor with media type "application/vnd.oci.image.manifest.v1+json" has an empty digest`)
	})
}

func TestCreateCopyOptsImageIndexes(t *testing.T) {
	ctx := context.Background()
	store := memory.New()

	otherArch := "arm64"
	if config.GetArch() == otherArch {
		otherArch = "amd64"
	}
	targetManifest := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString("target manifest"),
		Size:      1,
		Platform:  &ocispec.Platform{Architecture: config.GetArch(), OS: "linux"},
	}
	otherManifest := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString("other manifest"),
		Size:      1,
		Platform:  &ocispec.Platform{Architecture: otherArch, OS: "linux"},
	}
	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{
			targetManifest,
			otherManifest,
		},
	}
	indexDesc := pushTestJSON(t, ctx, store, ocispec.MediaTypeImageIndex, index)

	t.Run("bundle root selects target architecture", func(t *testing.T) {
		findSuccessors := CreateCopyOpts([]ocispec.Descriptor{targetManifest}, 1).FindSuccessors
		successors, err := findSuccessors(ctx, store, indexDesc)
		require.NoError(t, err)
		require.Equal(t, []ocispec.Descriptor{targetManifest}, successors)
	})

	t.Run("unannotated embedded image index preserves all manifests", func(t *testing.T) {
		layersToPull := []ocispec.Descriptor{indexDesc, targetManifest, otherManifest}
		findSuccessors := CreateCopyOpts(layersToPull, 1).FindSuccessors
		successors, err := findSuccessors(ctx, store, indexDesc)
		require.NoError(t, err)
		require.ElementsMatch(t, []ocispec.Descriptor{targetManifest, otherManifest}, successors)
	})
}

func pushTestBlob(t *testing.T, ctx context.Context, store content.Storage, mediaType string, data []byte) ocispec.Descriptor {
	t.Helper()
	desc := content.NewDescriptorFromBytes(mediaType, data)
	require.NoError(t, store.Push(ctx, desc, bytes.NewReader(data)))
	return desc
}

func pushTestJSON(t *testing.T, ctx context.Context, store content.Storage, mediaType string, value any) ocispec.Descriptor {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return pushTestBlob(t, ctx, store, mediaType, data)
}

func descriptorDigests(descriptors []ocispec.Descriptor) []digest.Digest {
	digests := make([]digest.Digest, 0, len(descriptors))
	for _, desc := range descriptors {
		digests = append(digests, desc.Digest)
	}
	return digests
}
