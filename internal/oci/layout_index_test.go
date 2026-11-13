// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/content"
)

func TestWriteIndex(t *testing.T) {
	root := t.TempDir()
	desc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, []byte("manifest"))
	idx := &ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{desc},
	}
	require.NoError(t, WriteIndex(filepath.Join(root, ocispec.ImageIndexFile), idx))

	data, err := os.ReadFile(filepath.Join(root, ocispec.ImageIndexFile))
	require.NoError(t, err)
	var got ocispec.Index
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, *idx, got)
	require.Len(t, got.Manifests, 1)
	assert.Equal(t, desc, got.Manifests[0])
}
