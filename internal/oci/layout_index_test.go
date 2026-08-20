// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"path/filepath"
	"testing"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/content"
)

func TestWriteIndexAndPackageRootDescriptor(t *testing.T) {
	root := t.TempDir()
	desc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, []byte("manifest"))
	idx := &ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{desc},
	}
	require.NoError(t, WriteIndex(filepath.Join(root, ocispec.ImageIndexFile), idx))

	got, err := packageRootDescriptor(root)
	require.NoError(t, err)
	assert.Equal(t, desc, got)
}

func TestPackageRootDescriptorRejectsMultipleRoots(t *testing.T) {
	root := t.TempDir()
	idx := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Manifests: []ocispec.Descriptor{{}, {}},
	}
	require.NoError(t, WriteIndex(filepath.Join(root, ocispec.ImageIndexFile), &idx))

	_, err := packageRootDescriptor(root)
	require.ErrorContains(t, err, "expected exactly 1")
	var countErr ManifestCountError
	require.ErrorAs(t, err, &countErr)
	assert.Equal(t, 2, countErr.Count)
	assert.Equal(t, 1, countErr.Want)
}
