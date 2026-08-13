// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

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
	desc, err := copySelectedPackage(t.Context(), pkgLayout, selected, store)
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
	require.NoError(t, os.MkdirAll(componentsDir, tempDirPerm))
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, layout.ZarfYAML), zarfYAML, tmpFilePerm))
	require.NoError(t, os.WriteFile(includedPath, []byte("included"), tmpFilePerm))
	require.NoError(t, os.WriteFile(excludedPath, []byte("excluded"), tmpFilePerm))
	checksums := fmt.Sprintf("%s %s\n%s %s\n",
		godigest.FromBytes([]byte("included")).Encoded(), filepath.ToSlash(filepath.Join(layout.ComponentsDir, "included.tar")),
		godigest.FromBytes([]byte("excluded")).Encoded(), filepath.ToSlash(filepath.Join(layout.ComponentsDir, "excluded.tar")),
	)
	checksumsPath := filepath.Join(dir, layout.Checksums)
	require.NoError(t, os.WriteFile(checksumsPath, []byte(checksums), tmpFilePerm))
	aggregate := godigest.FromBytes([]byte(checksums)).Encoded()
	zarfYAML = bytes.Replace(zarfYAML, []byte("metadata:\n  name: filtered"), []byte("metadata:\n  name: filtered\n  aggregateChecksum: "+aggregate), 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, layout.ZarfYAML), zarfYAML, tmpFilePerm))
}
