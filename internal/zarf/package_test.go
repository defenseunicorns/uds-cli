// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/cache"
	"github.com/defenseunicorns/uds-cli/internal/filesystem"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

func TestSelectedLayersMatchesDigestAndTitle(t *testing.T) {
	digest := godigest.FromString("same bytes")
	included := ocispec.Descriptor{
		Digest: digest,
		Annotations: map[string]string{
			ocispec.AnnotationTitle: "components/included.tar",
		},
	}
	excluded := ocispec.Descriptor{
		Digest: digest,
		Annotations: map[string]string{
			ocispec.AnnotationTitle: "components/excluded.tar",
		},
	}

	got := selectedLayers([]ocispec.Descriptor{included, excluded}, []ocispec.Descriptor{included})

	assert.Equal(t, []ocispec.Descriptor{included}, got)
}

func TestCopySelectedPackageWritesFilteredManifest(t *testing.T) {
	pkgDir := t.TempDir()
	writeFilteredPackageFiles(t, pkgDir)
	pkgLayout, err := layout.LoadFromDir(t.Context(), pkgDir, layout.PackageLayoutOptions{
		Filter:               BuildComponentFilter([]string{"included"}),
		IsPartial:            true,
		VerificationStrategy: layout.VerifyNever,
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()

	root, err := pkgLayout.Manifest()
	require.NoError(t, err)
	selected, partial, err := selectZarfLayers(t.Context(), root, pkgLayout, filters.Combine(filters.ForDeploy("included", false)))
	require.NoError(t, err)
	assert.True(t, partial)
	expected := selectedLayers(root.Layers, selected)

	store, err := udsoci.CreateStore(t.TempDir())
	require.NoError(t, err)
	desc, err := copySelectedPackage(t.Context(), pkgLayout, selected, store, "")
	require.NoError(t, err)

	manifestBytes, err := udsoci.FetchBytes(t.Context(), store, desc)
	require.NoError(t, err)
	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	assert.Equal(t, layerTitles(expected), layerTitles(manifest.Layers), "filtered package manifest must preserve upstream layer order")
	assert.Contains(t, layerTitles(manifest.Layers), layout.ZarfYAML)
	assert.Contains(t, layerTitles(manifest.Layers), layout.Checksums)
	assert.Contains(t, layerTitles(manifest.Layers), filepath.ToSlash(filepath.Join(layout.ComponentsDir, "included.tar")))
	assert.NotContains(t, layerTitles(manifest.Layers), filepath.ToSlash(filepath.Join(layout.ComponentsDir, "excluded.tar")))
}

func TestCopyPackageManifestCachesOnlySelectedImageBlobs(t *testing.T) {
	pkgDir := t.TempDir()
	included := []byte("included image layer")
	excluded := []byte("excluded image layer")
	other := []byte("not an image layer")
	files := map[string][]byte{
		filepath.ToSlash(filepath.Join(layout.ImagesBlobsDir, godigest.FromBytes(included).Encoded())): included,
		filepath.ToSlash(filepath.Join(layout.ImagesBlobsDir, godigest.FromBytes(excluded).Encoded())): excluded,
		"other.txt": other,
	}
	var checksums bytes.Buffer
	for path, data := range files {
		fullPath := filepath.Join(pkgDir, filepath.FromSlash(path))
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), filesystem.PrivateDirectoryMode))
		require.NoError(t, os.WriteFile(fullPath, data, filesystem.PrivateFileMode))
		_, err := fmt.Fprintf(&checksums, "%s %s\n", godigest.FromBytes(data).Encoded(), path)
		require.NoError(t, err)
	}
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, layout.Checksums), checksums.Bytes(), filesystem.PrivateFileMode))
	zarfYAML := fmt.Sprintf("kind: ZarfPackageConfig\nmetadata:\n  name: image-cache\n  version: 1.0.0\n  aggregateChecksum: %s\n", godigest.FromBytes(checksums.Bytes()).Encoded())
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, layout.ZarfYAML), []byte(zarfYAML), filesystem.PrivateFileMode))

	pkgLayout, err := layout.LoadFromDir(t.Context(), pkgDir, layout.PackageLayoutOptions{VerificationStrategy: layout.VerifyNever})
	require.NoError(t, err)
	root, manifest, err := packageManifest(t.Context(), pkgLayout)
	require.NoError(t, err)
	excludedDigest := godigest.FromBytes(excluded)
	manifest.Layers = slices.DeleteFunc(manifest.Layers, func(layer ocispec.Descriptor) bool {
		return layer.Digest == excludedDigest
	})
	store, err := udsoci.CreateStore(t.TempDir())
	require.NoError(t, err)
	cacheDir := t.TempDir()

	_, err = copyPackageManifest(t.Context(), pkgLayout, store, root, manifest, cacheDir)
	require.NoError(t, err)
	actual, err := os.ReadFile(filepath.Join(cacheDir, cache.LayersDirName, godigest.FromBytes(included).Encoded()))
	require.NoError(t, err)
	assert.Equal(t, included, actual)
	_, err = os.Stat(filepath.Join(cacheDir, cache.LayersDirName, excludedDigest.Encoded()))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(cacheDir, cache.LayersDirName, godigest.FromBytes(other).Encoded()))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func layerTitles(layers []ocispec.Descriptor) []string {
	titles := make([]string, 0, len(layers))
	for _, layer := range layers {
		titles = append(titles, layer.Annotations[ocispec.AnnotationTitle])
	}
	return titles
}

func writeFilteredPackageFiles(t *testing.T, dir string) {
	t.Helper()
	componentsDir := filepath.Join(dir, layout.ComponentsDir)
	require.NoError(t, os.MkdirAll(componentsDir, filesystem.PrivateDirectoryMode))
	zarfYAML := []byte(`kind: ZarfPackageConfig
metadata:
  name: filtered
  version: 1.0.0
components:
  - name: included
    required: true
  - name: excluded
`)
	includedPath := filepath.Join(componentsDir, "included.tar")
	excludedPath := filepath.Join(componentsDir, "excluded.tar")
	require.NoError(t, os.WriteFile(filepath.Join(dir, layout.ZarfYAML), zarfYAML, filesystem.PrivateFileMode))
	require.NoError(t, os.WriteFile(includedPath, []byte("included"), filesystem.PrivateFileMode))
	require.NoError(t, os.WriteFile(excludedPath, []byte("excluded"), filesystem.PrivateFileMode))
	checksums := fmt.Sprintf("%s %s\n%s %s\n",
		godigest.FromBytes([]byte("included")).Encoded(), filepath.ToSlash(filepath.Join(layout.ComponentsDir, "included.tar")),
		godigest.FromBytes([]byte("excluded")).Encoded(), filepath.ToSlash(filepath.Join(layout.ComponentsDir, "excluded.tar")),
	)
	checksumsPath := filepath.Join(dir, layout.Checksums)
	require.NoError(t, os.WriteFile(checksumsPath, []byte(checksums), filesystem.PrivateFileMode))
	aggregate := godigest.FromBytes([]byte(checksums)).Encoded()
	zarfYAML = bytes.Replace(zarfYAML, []byte("metadata:\n  name: filtered"), []byte("metadata:\n  name: filtered\n  aggregateChecksum: "+aggregate), 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, layout.ZarfYAML), zarfYAML, filesystem.PrivateFileMode))
}
