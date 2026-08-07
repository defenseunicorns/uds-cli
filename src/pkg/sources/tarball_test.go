// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package sources

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/mholt/archives"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"oras.land/oras-go/v2/content"
)

func TestTarballBundleLoadPackagePreservesManifestDigest(t *testing.T) {
	ctx := context.Background()
	packagePath, err := filepath.Abs(filepath.Join("..", "testdata", "zarf-package-real-simple-amd64-0.0.1.tar.zst"))
	require.NoError(t, err)
	packageLayout, err := layout.LoadFromTar(ctx, packagePath, layout.PackageLayoutOptions{
		Filter:               filters.Empty(),
		VerificationStrategy: layout.VerifyNever,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, packageLayout.Cleanup()) })

	manifestDesc, err := packageLayout.Resolve(ctx, packageLayout.Digest())
	require.NoError(t, err)
	manifestBytes, err := content.FetchAll(ctx, packageLayout, manifestDesc)
	require.NoError(t, err)
	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	blobs := map[string][]byte{
		filepath.Join(config.BlobsDir, manifestDesc.Digest.Encoded()): manifestBytes,
	}
	for _, layer := range manifest.Layers {
		layerBytes, err := content.FetchAll(ctx, packageLayout, layer)
		require.NoError(t, err)
		blobs[filepath.Join(config.BlobsDir, layer.Digest.Encoded())] = layerBytes
	}

	source := TarballBundle{
		PkgManifestDigest:       manifestDesc.Digest,
		BundleLocation:          writeBundleArchive(t, blobs),
		TmpDir:                  t.TempDir(),
		Pkg:                     types.Package{Name: "real-simple"},
		SkipSignatureValidation: true,
	}
	loadedLayout, _, err := source.LoadPackage(ctx, filters.Empty())
	require.NoError(t, err)
	require.Equal(t, manifestDesc.Digest.String(), loadedLayout.Digest())
	require.False(t, loadedLayout.IsPushable())
}

func TestExtractPackageManifestVerifiesDigest(t *testing.T) {
	manifestBytes, err := json.Marshal(oci.Manifest{Manifest: ocispec.Manifest{MediaType: ocispec.MediaTypeImageManifest}})
	require.NoError(t, err)

	t.Run("accepts matching content", func(t *testing.T) {
		manifestDigest := digest.FromBytes(manifestBytes)
		archive := writeBundleArchive(t, map[string][]byte{
			filepath.Join(config.BlobsDir, manifestDigest.Encoded()): manifestBytes,
		})
		file, err := os.Open(archive)
		require.NoError(t, err)
		defer file.Close()

		source := TarballBundle{PkgManifestDigest: manifestDigest}
		manifest, err := source.extractPackageManifest(context.Background(), file)
		require.NoError(t, err)
		require.Equal(t, ocispec.MediaTypeImageManifest, manifest.MediaType)
	})

	t.Run("rejects mismatched content", func(t *testing.T) {
		manifestDigest := digest.FromString("expected manifest")
		archive := writeBundleArchive(t, map[string][]byte{
			filepath.Join(config.BlobsDir, manifestDigest.Encoded()): manifestBytes,
		})
		file, err := os.Open(archive)
		require.NoError(t, err)
		defer file.Close()

		source := TarballBundle{PkgManifestDigest: manifestDigest}
		_, err = source.extractPackageManifest(context.Background(), file)
		require.ErrorContains(t, err, "package manifest digest mismatch")
	})
}

func TestExtractPackageVerifiesLayerDigest(t *testing.T) {
	expectedLayer := []byte("expected")
	tamperedLayer := []byte("tampered")
	layerDigest := digest.FromBytes(expectedLayer)
	manifestBytes, err := json.Marshal(oci.Manifest{Manifest: ocispec.Manifest{
		Layers: []ocispec.Descriptor{{
			MediaType: layout.ZarfLayerMediaTypeBlob,
			Digest:    layerDigest,
			Size:      int64(len(expectedLayer)),
			Annotations: map[string]string{
				ocispec.AnnotationTitle: layout.ZarfYAML,
			},
		}},
	}})
	require.NoError(t, err)
	manifestDigest := digest.FromBytes(manifestBytes)
	archive := writeBundleArchive(t, map[string][]byte{
		filepath.Join(config.BlobsDir, manifestDigest.Encoded()): manifestBytes,
		filepath.Join(config.BlobsDir, layerDigest.Encoded()):    tamperedLayer,
	})

	source := TarballBundle{
		PkgManifestDigest: manifestDigest,
		BundleLocation:    archive,
		TmpDir:            t.TempDir(),
	}
	_, err = source.extractPkgFromBundle()
	require.ErrorContains(t, err, "package layer \"zarf.yaml\" digest mismatch")
}

func writeBundleArchive(t *testing.T, blobs map[string][]byte) string {
	t.Helper()
	sourceDir := t.TempDir()
	fileMap := make(map[string]string, len(blobs))
	i := 0
	for archivePath, content := range blobs {
		diskPath := filepath.Join(sourceDir, string(rune('a'+i)))
		require.NoError(t, os.WriteFile(diskPath, content, 0o600))
		fileMap[diskPath] = archivePath
		i++
	}
	files, err := archives.FilesFromDisk(context.Background(), nil, fileMap)
	require.NoError(t, err)

	archivePath := filepath.Join(t.TempDir(), "bundle.tar.zst")
	archive, err := os.Create(archivePath)
	require.NoError(t, err)
	require.NoError(t, config.BundleArchiveFormat.Archive(context.Background(), archive, files))
	require.NoError(t, archive.Close())
	return archivePath
}
