// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package fetcher

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	"github.com/defenseunicorns/uds-cli/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
)

func TestLocalFetcherPreservesZarfManifest(t *testing.T) {
	ctx := context.Background()
	packagePath, err := filepath.Abs(filepath.Join("..", "..", "..", "test", "packages", "no-cluster", "real-simple", "zarf-package-real-simple-amd64-0.0.1.tar.zst"))
	require.NoError(t, err)

	expectedLayout, err := utils.LoadPackage(ctx, packagePath, packager.LoadOptions{
		Filter:               filters.Empty(),
		VerificationStrategy: layout.VerifyNever,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, expectedLayout.Cleanup()) })

	expectedRoot, err := expectedLayout.Resolve(ctx, expectedLayout.Digest())
	require.NoError(t, err)
	expectedManifestBytes, err := content.FetchAll(ctx, expectedLayout, expectedRoot)
	require.NoError(t, err)
	var expectedManifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(expectedManifestBytes, &expectedManifest))
	expectedConfigBytes, err := content.FetchAll(ctx, expectedLayout, expectedManifest.Config)
	require.NoError(t, err)

	store, err := ocistore.NewWithContext(ctx, t.TempDir())
	require.NoError(t, err)
	bundlePackage := types.Package{Name: "real-simple", Ref: "0.0.1", Path: packagePath}
	bundle := &types.UDSBundle{Packages: []types.Package{bundlePackage}}
	bundleRoot := &ocispec.Manifest{}
	f := localFetcher{
		pkg: bundlePackage,
		cfg: Config{
			Store:                   store,
			TmpDstDir:               t.TempDir(),
			BundleRootManifest:      bundleRoot,
			Bundle:                  bundle,
			SkipSignatureValidation: true,
		},
	}

	_, _, err = f.toBundle()
	require.NoError(t, err)
	require.Equal(t, "0.0.1@"+expectedRoot.Digest.String(), bundle.Packages[0].Ref)
	require.Len(t, bundleRoot.Layers, 1)
	require.Equal(t, expectedRoot.Digest, bundleRoot.Layers[0].Digest)
	require.Equal(t, layout.ZarfLayerMediaTypeBlob, bundleRoot.Layers[0].MediaType)
	require.Empty(t, bundleRoot.Layers[0].Annotations)
	require.Nil(t, bundleRoot.Layers[0].Platform)

	actualManifestBytes, err := content.FetchAll(ctx, store, expectedRoot)
	require.NoError(t, err)
	require.Equal(t, expectedManifestBytes, actualManifestBytes)
	actualConfigBytes, err := content.FetchAll(ctx, store, expectedManifest.Config)
	require.NoError(t, err)
	require.Equal(t, expectedConfigBytes, actualConfigBytes)

	requested := append([]ocispec.Descriptor{expectedManifest.Config}, expectedManifest.Layers...)
	successors, err := boci.CreateCopyOpts(requested, 1).FindSuccessors(ctx, store, bundleRoot.Layers[0])
	require.NoError(t, err)
	require.ElementsMatch(t, requested, successors)
}
