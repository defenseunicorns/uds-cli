// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/defenseunicorns/pkg/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
)

func TestForceUploadRepoAlwaysReportsContentMissing(t *testing.T) {
	exists, err := (&forceUploadRepo{}).Exists(context.Background(), ocispec.Descriptor{})

	require.NoError(t, err)
	require.False(t, exists)
}

func TestGetZarfLayersIncludesManifestConfig(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir()
	store, err := ocistore.NewWithContext(ctx, storeDir)
	require.NoError(t, err)

	configBytes := []byte(`{"metadata":{"name":"example"}}`)
	configDesc := content.NewDescriptorFromBytes(layout.ZarfConfigMediaType, configBytes)
	require.NoError(t, store.Push(ctx, configDesc, bytes.NewReader(configBytes)))
	layerBytes := []byte("layer")
	layerDesc := content.NewDescriptorFromBytes(layout.ZarfLayerMediaTypeBlob, layerBytes)
	require.NoError(t, store.Push(ctx, layerDesc, bytes.NewReader(layerBytes)))

	manifestBytes, err := json.Marshal(oci.Manifest{Manifest: ocispec.Manifest{
		Config: configDesc,
		Layers: []ocispec.Descriptor{layerDesc},
	}})
	require.NoError(t, err)
	manifestDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestBytes)
	require.NoError(t, store.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)))

	provider := tarballBundleProvider{ctx: ctx, dst: storeDir}
	descs, size, err := provider.getZarfLayers(store, manifestDesc)
	require.NoError(t, err)
	require.ElementsMatch(t, []ocispec.Descriptor{layerDesc, configDesc}, descs)
	require.Equal(t, layerDesc.Size+configDesc.Size, size)
}
