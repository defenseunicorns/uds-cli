// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package fetcher

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
)

func TestRemoteFetcherPreservesCachedPackageManifest(t *testing.T) {
	ctx := context.Background()
	manifest := oci.Manifest{Manifest: ocispec.Manifest{MediaType: ocispec.MediaTypeImageManifest}}
	manifestBytes, err := json.Marshal(&manifest)
	require.NoError(t, err)
	var fetchedManifest oci.Manifest
	require.NoError(t, json.Unmarshal(manifestBytes, &fetchedManifest))
	sourceDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestBytes)
	sourceDesc.Annotations = map[string]string{ocispec.AnnotationTitle: "package"}

	store, err := ocistore.NewWithContext(ctx, t.TempDir())
	require.NoError(t, err)
	bundleRoot := &ocispec.Manifest{}
	f := remoteFetcher{
		pkg:             types.Package{Name: "example"},
		pkgRootManifest: &fetchedManifest,
		cfg: Config{
			Store:              store,
			BundleRootManifest: bundleRoot,
		},
	}

	descs, err := f.copyRemotePkgLayers(nil)
	require.NoError(t, err)
	require.Len(t, descs, 1)
	require.Equal(t, sourceDesc.Digest, descs[0].Digest)
	require.Len(t, bundleRoot.Layers, 1)
	require.Equal(t, sourceDesc.Digest, bundleRoot.Layers[0].Digest)
	require.Equal(t, layout.ZarfLayerMediaTypeBlob, bundleRoot.Layers[0].MediaType)
	require.Empty(t, bundleRoot.Layers[0].Annotations)

	storedBytes, err := content.FetchAll(ctx, store, sourceDesc)
	require.NoError(t, err)
	require.Equal(t, manifestBytes, storedBytes)
}
