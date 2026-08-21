// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadPackageZarfNames(t *testing.T) {
	manifests, blobs := packageMetadataFixture(t, "bundle-label", "deployed-zarf-name")
	fetch := func(_ context.Context, desc ocispec.Descriptor) ([]byte, error) {
		return blobs[desc.Digest], nil
	}
	names, err := readPackageZarfNames(t.Context(), manifests, fetch)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"bundle-label": "deployed-zarf-name"}, names)
}
func TestReadPackageZarfNamesRequiresMetadataName(t *testing.T) {
	manifests, blobs := packageMetadataFixture(t, "bundle-label", "")
	fetch := func(_ context.Context, desc ocispec.Descriptor) ([]byte, error) {
		return blobs[desc.Digest], nil
	}
	_, err := readPackageZarfNames(t.Context(), manifests, fetch)
	var target MissingZarfPackageNameError
	require.ErrorAs(t, err, &target)
	assert.Equal(t, "bundle-label", target.Package)
}
func packageMetadataFixture(t *testing.T, bundleName, zarfName string) (map[string]ocispec.Descriptor, map[digest.Digest][]byte) {
	t.Helper()
	zarfYAML := []byte("metadata:\n  name: " + zarfName + "\nbuild:\n  signed: true\n")
	zarfDesc := ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Digest:    digest.FromBytes(zarfYAML),
		Size:      int64(len(zarfYAML)),
		Annotations: map[string]string{
			ocispec.AnnotationTitle: "zarf.yaml",
		},
	}
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Layers:    []ocispec.Descriptor{zarfDesc},
	}
	manifestBytes, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestBytes),
		Size:      int64(len(manifestBytes)),
	}
	return map[string]ocispec.Descriptor{bundleName: manifestDesc}, map[digest.Digest][]byte{
		manifestDesc.Digest: manifestBytes,
		zarfDesc.Digest:     zarfYAML,
	}
}
