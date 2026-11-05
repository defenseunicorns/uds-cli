// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/mholt/archives"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
)

func TestBundleProviderArchiveFormat(t *testing.T) {
	manifestJSON := []byte(`{"schemaVersion":2}`)
	manifestDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestJSON)
	indexJSON, err := json.Marshal(ocispec.Index{Manifests: []ocispec.Descriptor{manifestDesc}})
	require.NoError(t, err)

	for _, tt := range []struct {
		name      string
		indexPath string
		indexJSON string
		wantError string
	}{
		{"legacy bundle", "index.json", string(indexJSON), ""},
		{"next bundle", "oci/index.json", `{}`, "CLI_FEATURES=NextMode=true uds bundle inspect <bundle-reference>"},
		{"missing index", "other.json", `{}`, "expected only one manifest in index.json, found 0"},
		{"empty index", "index.json", `{"manifests":[]}`, "expected only one manifest in index.json, found 0"},
		{"multiple manifests", "index.json", `{"manifests":[{},{}]}`, "expected only one manifest in index.json, found 2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			indexPath := filepath.Join(t.TempDir(), "index.json")
			require.NoError(t, os.WriteFile(indexPath, []byte(tt.indexJSON), 0o600))
			manifestPath := filepath.Join(t.TempDir(), "manifest.json")
			require.NoError(t, os.WriteFile(manifestPath, manifestJSON, 0o600))
			files, err := archives.FilesFromDisk(ctx, nil, map[string]string{
				indexPath:    tt.indexPath,
				manifestPath: filepath.Join(config.BlobsDir, manifestDesc.Digest.Encoded()),
			})
			require.NoError(t, err)
			archivePath := filepath.Join(t.TempDir(), "uds-bundle-test-amd64-1.0.0.tar.zst")
			out, err := os.Create(archivePath)
			require.NoError(t, err)
			require.NoError(t, config.BundleArchiveFormat.Archive(ctx, out, files))
			require.NoError(t, out.Close())

			_, err = NewBundleProvider(archivePath, t.TempDir())
			if tt.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantError)
		})
	}
}

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
