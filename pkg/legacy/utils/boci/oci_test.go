// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package boci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/utils"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
)

func TestGetIndexRejectsNextBundles(t *testing.T) {
	for _, tt := range []struct {
		name  string
		index string
		next  bool
	}{
		{"legacy", `{"schemaVersion":2,"manifests":[]}`, false},
		{"child", `{"schemaVersion":2,"artifactType":"application/vnd.defenseunicorns.uds.bundle.v1","manifests":[]}`, true},
		{"root", `{"schemaVersion":2,"manifests":[{"artifactType":"application/vnd.defenseunicorns.uds.bundle.v1"}]}`, true},
		{"definition", `{"schemaVersion":2,"manifests":[{"artifactType":"application/vnd.defenseunicorns.uds.bundle.definition.v1"}]}`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", ocispec.MediaTypeImageIndex)
				w.Header().Set("Docker-Content-Digest", digest.FromString(tt.index).String())
				w.Header().Set("Content-Length", strconv.Itoa(len(tt.index)))
				if r.Method != http.MethodHead {
					_, _ = fmt.Fprint(w, tt.index)
				}
			}))
			t.Cleanup(server.Close)
			remote, err := oci.NewOrasRemote(strings.TrimPrefix(server.URL, "http://")+"/bundle:tag", ocispec.Platform{}, oci.WithPlainHTTP(true))
			require.NoError(t, err)
			index, err := GetIndex(remote, "tag")
			if tt.next {
				require.ErrorIs(t, err, utils.ErrNextBundle)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, index)
		})
	}
}

func TestFilterComponentsV1beta1RequiredAndOptional(t *testing.T) {
	pkg, err := utils.ReadPackageYAMLBytes([]byte(`
apiVersion: zarf.dev/v1beta1
kind: ZarfPackageConfig
components:
  - name: required
  - name: optional
    optional: true
`))
	require.NoError(t, err)

	filtered, err := filterComponents(pkg, nil)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "required", filtered[0].Name)

	filtered, err = filterComponents(pkg, []string{"optional"})
	require.NoError(t, err)
	require.Len(t, filtered, 2)
	assert.ElementsMatch(t, []string{"required", "optional"}, []string{filtered[0].Name, filtered[1].Name})
}

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
		components []api.Component
		wantImages []string
	}{
		{
			name: "only imageArchives",
			components: []api.Component{
				{
					Name: "hookos-artifacts",
					ImageArchives: []api.ImageArchive{
						{Path: "image.tar", Images: []api.ArchiveImage{{Name: archiveImg}}},
					},
				},
			},
			wantImages: []string{archiveImg},
		},
		{
			name: "only images",
			components: []api.Component{
				{Name: "regular", Images: []api.Image{{Name: regularImg}}},
			},
			wantImages: []string{regularImg},
		},
		{
			name: "both images and imageArchives",
			components: []api.Component{
				{
					Name:   "mixed",
					Images: []api.Image{{Name: regularImg}},
					ImageArchives: []api.ImageArchive{
						{Path: "image.tar", Images: []api.ArchiveImage{{Name: archiveImg}}},
					},
				},
			},
			wantImages: []string{regularImg, archiveImg},
		},
		{
			name: "no matching images",
			components: []api.Component{
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
