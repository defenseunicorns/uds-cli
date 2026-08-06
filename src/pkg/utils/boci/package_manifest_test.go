// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package boci

import (
	"context"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
)

func TestPushPackageManifestPreservesSourceBytes(t *testing.T) {
	ctx := context.Background()
	manifestBytes := []byte("{\n  \"schemaVersion\": 2,\n  \"mediaType\": \"application/vnd.oci.image.manifest.v1+json\",\n  \"x-extension\": {\"preserved\": true}\n}\n")
	sourceDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestBytes)
	sourceDesc.Annotations = map[string]string{ocispec.AnnotationTitle: "package"}
	sourceDesc.Platform = &ocispec.Platform{OS: "multi", Architecture: "amd64"}
	store := memory.New()

	blobDesc, err := PushPackageManifest(ctx, store, sourceDesc, manifestBytes)
	require.NoError(t, err)
	require.Equal(t, sourceDesc.Digest, blobDesc.Digest)
	require.Equal(t, sourceDesc.Size, blobDesc.Size)
	require.Equal(t, layout.ZarfLayerMediaTypeBlob, blobDesc.MediaType)
	require.Empty(t, blobDesc.Annotations)
	require.Nil(t, blobDesc.Platform)

	storedBytes, err := content.FetchAll(ctx, store, blobDesc)
	require.NoError(t, err)
	require.Equal(t, manifestBytes, storedBytes)
	_, err = PushPackageManifest(ctx, store, sourceDesc, manifestBytes)
	require.NoError(t, err)
}

func TestPushPackageManifestRejectsMismatchedSource(t *testing.T) {
	manifestBytes := []byte(`{"schemaVersion":2}`)
	sourceDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, []byte(`{"schemaVersion":1}`))

	_, err := PushPackageManifest(context.Background(), memory.New(), sourceDesc, manifestBytes)
	require.ErrorContains(t, err, "package manifest content does not match source descriptor")
}
