// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	packageoci "github.com/defenseunicorns/pkg/oci"
	godigest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/content/memory"
)

func TestIsImageManifestMediaType(t *testing.T) {
	assert.True(t, IsImageManifestMediaType(ocispec.MediaTypeImageManifest))
	assert.True(t, IsImageManifestMediaType(mediaTypeDockerManifest))
	assert.False(t, IsImageManifestMediaType(ocispec.MediaTypeImageIndex))
}

func TestPushBytesStoresAnnotatedContent(t *testing.T) {
	store := memory.New()
	data := []byte("bundle")
	annotations := map[string]string{ocispec.AnnotationTitle: "bundle.uds.hcl"}

	desc, err := PushBytes(t.Context(), store, MediaTypeBundleHCL, data, annotations)
	require.NoError(t, err)

	assert.Equal(t, MediaTypeBundleHCL, desc.MediaType)
	assert.Equal(t, annotations, desc.Annotations)
	got, err := FetchBytes(t.Context(), store, desc)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestPushManifestBytesSetsArtifactType(t *testing.T) {
	store := memory.New()
	manifestBytes := []byte(`{"schemaVersion":2}`)

	desc, err := PushManifestBytes(t.Context(), store, ocispec.MediaTypeImageManifest, MediaTypeBundleDefinition, manifestBytes)
	require.NoError(t, err)

	assert.Equal(t, ocispec.MediaTypeImageManifest, desc.MediaType)
	assert.Equal(t, MediaTypeBundleDefinition, desc.ArtifactType)
	got, err := FetchBytes(t.Context(), store, desc)
	require.NoError(t, err)
	assert.Equal(t, manifestBytes, got)
}

func TestFetchBytesWrapsDescriptorErrors(t *testing.T) {
	store := memory.New()
	desc := ocispec.Descriptor{Digest: godigest.FromString("missing"), Size: 7}

	_, err := FetchBytes(t.Context(), store, desc)

	require.Error(t, err)
	assert.ErrorContains(t, err, "fetching "+desc.Digest.String())
}

func TestFetchBytesRejectsOversizedDescriptor(t *testing.T) {
	store := memory.New()
	desc := ocispec.Descriptor{Digest: godigest.FromString("metadata"), Size: MaxFetchBytesSize + 1}

	_, err := FetchBytes(t.Context(), store, desc)

	require.Error(t, err)
	require.ErrorContains(t, err, "larger than the")
	var sizeErr DescriptorTooLargeError
	require.ErrorAs(t, err, &sizeErr)
	assert.Equal(t, desc.Digest, sizeErr.Digest)
	assert.Equal(t, desc.Size, sizeErr.Size)
	assert.Equal(t, int64(MaxFetchBytesSize), sizeErr.Limit)
}

func TestEnsureTagAvailableRejectsExistingTag(t *testing.T) {
	store := memory.New()
	desc, err := PushBytes(t.Context(), store, MediaTypeBundleHCL, []byte("bundle"), nil)
	require.NoError(t, err)
	require.NoError(t, store.Tag(t.Context(), desc, "v1"))

	assert.NoError(t, EnsureTagAvailable(t.Context(), store, "missing"))
	err = EnsureTagAvailable(t.Context(), store, "v1")
	require.ErrorContains(t, err, "target tag \"v1\" already exists")
	var tagErr TargetTagExistsError
	require.ErrorAs(t, err, &tagErr)
	assert.Equal(t, "v1", tagErr.Tag)
}

func TestBundleChildDescriptorSetsBundlePlatform(t *testing.T) {
	desc := BundleChildDescriptor(NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, []byte("index")), "amd64")

	assert.Equal(t, MediaTypeBundle, desc.ArtifactType)
	require.NotNil(t, desc.Platform)
	assert.Equal(t, "amd64", desc.Platform.Architecture)
	assert.Equal(t, packageoci.MultiOS, desc.Platform.OS)
}

func TestPushReferenceBytesUsesManifestReferencePusher(t *testing.T) {
	target := &referencePushTarget{Store: memory.New()}
	data := []byte(`{"schemaVersion":2}`)
	desc := NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, data)

	require.NoError(t, PushReferenceBytes(t.Context(), target, desc, data, "v1"))

	assert.True(t, target.called)
	resolved, err := target.Resolve(t.Context(), "v1")
	require.NoError(t, err)
	assert.Equal(t, desc.Digest, resolved.Digest)
}

type referencePushTarget struct {
	*memory.Store
	called bool
}

func (t *referencePushTarget) PushReference(ctx context.Context, expected ocispec.Descriptor, r io.Reader, reference string) error {
	t.called = true
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if err := PushDescriptorBytes(ctx, t.Store, expected, data); err != nil {
		return err
	}
	return t.Tag(ctx, expected, reference)
}

func TestPublishBundleRootIndexSkipsAlreadyPublishedRoot(t *testing.T) {
	store := memory.New()
	child := BundleChildDescriptor(NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, []byte("child")), "amd64")
	rootBytes, rootDesc, _, err := mergeRootIndex(t.Context(), store, "v1", child)
	require.NoError(t, err)
	require.NoError(t, PushDescriptorBytes(t.Context(), store, rootDesc, rootBytes))
	require.NoError(t, store.Tag(t.Context(), rootDesc, "v1"))
	target := &failingReferencePushTarget{Store: store}

	require.NoError(t, PublishBundleRootIndex(t.Context(), target, "v1", child))
	assert.False(t, target.called)
}

type failingReferencePushTarget struct {
	*memory.Store
	called bool
}

func (t *failingReferencePushTarget) PushReference(context.Context, ocispec.Descriptor, io.Reader, string) error {
	t.called = true
	return assert.AnError
}

func TestPublishBundleRootIndexTagsRootAndPreservesOtherArchitectures(t *testing.T) {
	store := memory.New()
	existingChild := BundleChildDescriptor(NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, []byte("arm child")), "arm64")
	existingRootBytes, existingRootDesc, _, err := mergeRootIndex(t.Context(), store, "v1", existingChild)
	require.NoError(t, err)
	require.NoError(t, PushDescriptorBytes(t.Context(), store, existingRootDesc, existingRootBytes))
	require.NoError(t, store.Tag(t.Context(), existingRootDesc, "v1"))

	newChild := BundleChildDescriptor(NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, []byte("amd child")), "amd64")
	require.NoError(t, PublishBundleRootIndex(t.Context(), store, "v1", newChild))

	rootDesc, err := store.Resolve(t.Context(), "v1")
	require.NoError(t, err)
	rootBytes, err := FetchBytes(t.Context(), store, rootDesc)
	require.NoError(t, err)
	var root ocispec.Index
	require.NoError(t, json.Unmarshal(rootBytes, &root))
	require.Len(t, root.Manifests, 2)
	assert.Equal(t, "amd64", root.Manifests[0].Platform.Architecture)
	assert.Equal(t, "arm64", root.Manifests[1].Platform.Architecture)
}

func TestCopyGraphCopiesManifestContent(t *testing.T) {
	ctx := context.Background()
	src := memory.New()
	dst := memory.New()

	config := pushTestContent(t, ctx, src, "application/vnd.test.config", []byte("{}"))
	layer := pushTestContent(t, ctx, src, "application/vnd.test.layer", []byte("layer"))
	manifestBytes, err := json.Marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    config,
		Layers:    []ocispec.Descriptor{layer},
	})
	require.NoError(t, err)
	root := NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestBytes)
	require.NoError(t, PushDescriptorBytes(ctx, src, root, manifestBytes))

	require.NoError(t, CopyGraph(ctx, src, dst, root))

	gotManifest, err := FetchBytes(ctx, dst, root)
	require.NoError(t, err)
	assert.Equal(t, manifestBytes, gotManifest)
	gotLayer, err := FetchBytes(ctx, dst, layer)
	require.NoError(t, err)
	assert.Equal(t, []byte("layer"), gotLayer)
}

func pushTestContent(t *testing.T, ctx context.Context, store *memory.Store, mediaType string, data []byte) ocispec.Descriptor {
	t.Helper()
	desc := NewDescriptorFromBytes(mediaType, data)
	require.NoError(t, PushDescriptorBytes(ctx, store, desc, data))
	return desc
}
