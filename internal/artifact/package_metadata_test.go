// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/content"
)

func TestReadPackageZarfNames(t *testing.T) {
	manifests, blobs, _ := packageMetadataFixture(t, "bundle-label", []byte("metadata:\n  name: deployed-zarf-name\nbuild:\n  signed: true\n"), true, 0)

	names, err := readPackageZarfNames(t.Context(), manifests, blobFetcher(blobs, nil))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"bundle-label": "deployed-zarf-name"}, names)
}

func TestReadPackageZarfNamesRequiresMetadataName(t *testing.T) {
	t.Run("empty metadata name", func(t *testing.T) {
		manifests, blobs, _ := packageMetadataFixture(t, "bundle-label", []byte("metadata:\n  name: \n"), true, 0)

		_, err := readPackageZarfNames(t.Context(), manifests, blobFetcher(blobs, nil))
		var target MissingZarfPackageNameError
		require.ErrorAs(t, err, &target)
		assert.Equal(t, "bundle-label", target.Package)
	})

	t.Run("missing zarf yaml", func(t *testing.T) {
		manifests, blobs, _ := packageMetadataFixture(t, "bundle-label", nil, false, 0)

		_, err := readPackageZarfNames(t.Context(), manifests, blobFetcher(blobs, nil))
		var target MissingZarfPackageNameError
		require.ErrorAs(t, err, &target)
		assert.Equal(t, "bundle-label", target.Package)
	})
}

func TestFetchZarfPackageClassifiesMalformedYAML(t *testing.T) {
	manifests, blobs, _ := packageMetadataFixture(t, "bundle-label", []byte("metadata:\n  name: [\n"), true, 0)

	_, _, err := fetchZarfPackage(t.Context(), "bundle-label", manifests["bundle-label"], blobFetcher(blobs, nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrParsingZarfYAML)
	assert.NotErrorIs(t, err, ErrFetchingZarfYAML)
}

func TestFetchZarfPackageClassifiesFetchFailure(t *testing.T) {
	manifests, blobs, zarfDesc := packageMetadataFixture(t, "bundle-label", []byte("metadata:\n  name: test\n"), true, 0)
	fetchErr := errors.New("storage unavailable")
	fetcher := content.FetcherFunc(func(_ context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
		if desc.Digest == zarfDesc.Digest {
			return nil, fetchErr
		}
		return readerForDescriptor(blobs, desc)
	})

	_, _, err := fetchZarfPackage(t.Context(), "bundle-label", manifests["bundle-label"], fetcher)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchingZarfYAML)
	assert.ErrorIs(t, err, fetchErr)
	assert.NotErrorIs(t, err, ErrParsingZarfYAML)
}

func TestFetchZarfPackageRejectsOversizedYAMLBeforeReading(t *testing.T) {
	declaredSize := int64(udsoci.MaxFetchBytesSize + 1)
	manifests, blobs, zarfDesc := packageMetadataFixture(t, "bundle-label", []byte("metadata:\n  name: test\n"), true, declaredSize)
	zarfReads := 0
	fetcher := blobFetcher(blobs, func(desc ocispec.Descriptor) {
		if desc.Digest == zarfDesc.Digest {
			zarfReads++
		}
	})

	_, _, err := fetchZarfPackage(t.Context(), "bundle-label", manifests["bundle-label"], fetcher)
	var target udsoci.DescriptorTooLargeError
	require.ErrorAs(t, err, &target)
	assert.ErrorIs(t, err, ErrFetchingZarfYAML)
	assert.Equal(t, 0, zarfReads)
}

func TestFetchZarfPackageUsesZarfMultiDocParsingAndMigrations(t *testing.T) {
	zarfYAML := []byte(`apiVersion: unsupported.example/v1
metadata:
  name: ignored
---
apiVersion: zarf.dev/v1alpha1
metadata:
  name: migrated-package
components:
  - name: example
    scripts:
      before:
        - echo migrated
`)
	manifests, blobs, _ := packageMetadataFixture(t, "bundle-label", zarfYAML, true, 0)

	pkg, found, err := fetchZarfPackage(t.Context(), "bundle-label", manifests["bundle-label"], blobFetcher(blobs, nil))
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "migrated-package", pkg.Metadata.Name)
	require.Len(t, pkg.Components, 1)
	require.Len(t, pkg.Components[0].Actions.OnDeploy.Before, 1)
	assert.Equal(t, "echo migrated", pkg.Components[0].Actions.OnDeploy.Before[0].Cmd)
}

func TestInspectPackageSignatureUsesZarfMetadata(t *testing.T) {
	t.Run("signed package", func(t *testing.T) {
		manifests, blobs, _ := packageMetadataFixture(t, "bundle-label", []byte("metadata:\n  name: test\nbuild:\n  signed: true\n"), true, 0)
		entry := manifests["bundle-label"]
		entry.Annotations = map[string]string{udsoci.AnnotationPackageName: "bundle-label"}
		idx := ocispec.Index{Manifests: []ocispec.Descriptor{entry}}

		summary, err := inspectPackageSignature(t.Context(), idx, spec.Package{Name: "bundle-label"}, blobFetcher(blobs, nil))
		require.NoError(t, err)
		assert.Equal(t, PackageSigningStatusSigned, summary.Signed)
	})

	t.Run("missing zarf yaml", func(t *testing.T) {
		manifests, blobs, _ := packageMetadataFixture(t, "bundle-label", nil, false, 0)
		entry := manifests["bundle-label"]
		entry.Annotations = map[string]string{udsoci.AnnotationPackageName: "bundle-label"}
		idx := ocispec.Index{Manifests: []ocispec.Descriptor{entry}}

		summary, err := inspectPackageSignature(t.Context(), idx, spec.Package{Name: "bundle-label"}, blobFetcher(blobs, nil))
		require.NoError(t, err)
		assert.Equal(t, PackageSigningStatusUnknown, summary.Signed)
	})
}

func packageMetadataFixture(t *testing.T, bundleName string, zarfYAML []byte, includeZarf bool, declaredZarfSize int64) (map[string]ocispec.Descriptor, map[digest.Digest][]byte, ocispec.Descriptor) {
	t.Helper()
	blobs := map[digest.Digest][]byte{}
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
	}
	var zarfDesc ocispec.Descriptor
	if includeZarf {
		if declaredZarfSize == 0 {
			declaredZarfSize = int64(len(zarfYAML))
		}
		zarfDesc = ocispec.Descriptor{
			MediaType: "application/octet-stream",
			Digest:    digest.FromBytes(zarfYAML),
			Size:      declaredZarfSize,
			Annotations: map[string]string{
				ocispec.AnnotationTitle: "zarf.yaml",
			},
		}
		manifest.Layers = []ocispec.Descriptor{zarfDesc}
		blobs[zarfDesc.Digest] = zarfYAML
	}
	manifestBytes, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestBytes),
		Size:      int64(len(manifestBytes)),
	}
	blobs[manifestDesc.Digest] = manifestBytes
	return map[string]ocispec.Descriptor{bundleName: manifestDesc}, blobs, zarfDesc
}

func blobFetcher(blobs map[digest.Digest][]byte, onFetch func(ocispec.Descriptor)) content.Fetcher {
	return content.FetcherFunc(func(_ context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
		if onFetch != nil {
			onFetch(desc)
		}
		return readerForDescriptor(blobs, desc)
	})
}

func readerForDescriptor(blobs map[digest.Digest][]byte, desc ocispec.Descriptor) (io.ReadCloser, error) {
	data, ok := blobs[desc.Digest]
	if !ok {
		return nil, fmt.Errorf("blob %s not found", desc.Digest)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
