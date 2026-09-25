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
	"maps"
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
		require.Equal(t, "bundle-label", target.Package)
	})
}

type blobBatchFetcher struct {
	blobs map[digest.Digest][]byte
}

type failingBatchFetcher struct {
	err        error
	fetches    int
	batchReads int
}

type descriptorFailingBatchFetcher struct {
	blobBatchFetcher
	failingDigest digest.Digest
	err           error
}

type postVisitFailingBatchFetcher struct {
	blobBatchFetcher
	failingDigest digest.Digest
	err           error
	failed        bool
}

func (f *failingBatchFetcher) Fetch(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
	f.fetches++
	return nil, f.err
}

func (f *failingBatchFetcher) FetchBatch(context.Context, []ocispec.Descriptor, func(ocispec.Descriptor, []byte, error) error) error {
	f.batchReads++
	return f.err
}

func (f blobBatchFetcher) Fetch(_ context.Context, descriptor ocispec.Descriptor) (io.ReadCloser, error) {
	return readerForDescriptor(f.blobs, descriptor)
}

func (f descriptorFailingBatchFetcher) FetchBatch(ctx context.Context, descriptors []ocispec.Descriptor, visit func(ocispec.Descriptor, []byte, error) error) error {
	return f.blobBatchFetcher.FetchBatch(ctx, descriptors, func(descriptor ocispec.Descriptor, data []byte, fetchErr error) error {
		if descriptor.Digest == f.failingDigest {
			return visit(descriptor, nil, f.err)
		}
		return visit(descriptor, data, fetchErr)
	})
}

func (f *postVisitFailingBatchFetcher) FetchBatch(ctx context.Context, descriptors []ocispec.Descriptor, visit func(ocispec.Descriptor, []byte, error) error) error {
	if err := f.blobBatchFetcher.FetchBatch(ctx, descriptors, visit); err != nil {
		return err
	}
	if !f.failed {
		for _, descriptor := range descriptors {
			if descriptor.Digest == f.failingDigest {
				f.failed = true
				return f.err
			}
		}
	}
	return nil
}

func (f blobBatchFetcher) FetchBatch(_ context.Context, descriptors []ocispec.Descriptor, visit func(ocispec.Descriptor, []byte, error) error) error {
	for _, descriptor := range descriptors {
		data, ok := f.blobs[descriptor.Digest]
		if !ok {
			err := fmt.Errorf("blob %s not found", descriptor.Digest)
			if err := visit(descriptor, nil, err); err != nil {
				return err
			}
			continue
		}
		if err := visit(descriptor, data, nil); err != nil {
			return err
		}
	}
	return nil
}

func readZarfPackageMetadataFixture(ctx context.Context, packageNames []string, manifests map[string]ocispec.Descriptor, fetcher content.Fetcher) (map[string]zarfPackageMetadata, bool, error) {
	states := make([]bundlePackageMetadataState, 0, len(packageNames))
	for _, packageName := range packageNames {
		descriptor := manifests[packageName]
		states = append(states, bundlePackageMetadataState{packageName: packageName, manifest: &descriptor})
	}
	return readZarfPackageMetadataBatch(ctx, states, fetcher)
}

func TestZarfPackageMetadataBatchMatchesOrdinaryParsingErrors(t *testing.T) {
	tests := []struct {
		name        string
		zarfYAML    []byte
		alterRoot   func(*ocispec.Manifest)
		rootBytes   []byte
		assertError func(*testing.T, error)
	}{
		{
			name:      "malformed package root manifest",
			zarfYAML:  []byte("metadata:\n  name: test\n"),
			rootBytes: []byte("{"),
			assertError: func(t *testing.T, err error) {
				t.Helper()
				require.ErrorIs(t, err, ErrParsingPackageManifest)
			},
		},
		{
			name:      "unsupported package root schema",
			zarfYAML:  []byte("metadata:\n  name: test\n"),
			alterRoot: func(root *ocispec.Manifest) { root.SchemaVersion = 1 },
			assertError: func(t *testing.T, err error) {
				t.Helper()
				var target UnsupportedSchemaVersionError
				require.ErrorAs(t, err, &target)
				assert.Equal(t, "package manifest", target.Artifact)
				assert.Equal(t, 1, target.Version)
			},
		},
		{
			name:      "unsupported package root media type",
			zarfYAML:  []byte("metadata:\n  name: test\n"),
			alterRoot: func(root *ocispec.Manifest) { root.MediaType = "application/unsupported" },
			assertError: func(t *testing.T, err error) {
				t.Helper()
				var target UnsupportedMediaTypeError
				require.ErrorAs(t, err, &target)
				assert.Equal(t, "package manifest", target.Artifact)
				assert.Equal(t, "application/unsupported", target.MediaType)
			},
		},
		{
			name:     "malformed zarf metadata",
			zarfYAML: []byte("metadata:\n  name: [\n"),
			assertError: func(t *testing.T, err error) {
				t.Helper()
				require.ErrorIs(t, err, ErrParsingZarfYAML)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const packageName = "bundle-label"
			manifests, blobs, _ := packageMetadataFixture(t, packageName, tt.zarfYAML, true, 0)
			if tt.rootBytes != nil || tt.alterRoot != nil {
				manifestDescriptor := manifests[packageName]
				manifestBytes := tt.rootBytes
				if manifestBytes == nil {
					var root ocispec.Manifest
					require.NoError(t, json.Unmarshal(blobs[manifestDescriptor.Digest], &root))
					tt.alterRoot(&root)
					var err error
					manifestBytes, err = json.Marshal(root)
					require.NoError(t, err)
				}
				delete(blobs, manifestDescriptor.Digest)
				manifestDescriptor.Digest = digest.FromBytes(manifestBytes)
				manifestDescriptor.Size = int64(len(manifestBytes))
				manifests[packageName] = manifestDescriptor
				blobs[manifestDescriptor.Digest] = manifestBytes
			}

			fetcher := blobBatchFetcher{blobs: blobs}
			_, _, ordinaryErr := fetchZarfPackage(t.Context(), packageName, manifests[packageName], fetcher)
			tt.assertError(t, ordinaryErr)

			_, batched, err := readZarfPackageMetadataFixture(t.Context(), []string{packageName}, manifests, fetcher)
			require.True(t, batched)
			var packageErr packageMetadataError
			require.ErrorAs(t, err, &packageErr)
			assert.Equal(t, packageName, packageErr.Package)
			tt.assertError(t, packageErr.Err)
		})
	}
}

func TestZarfPackageMetadataBatchRetainsMissingDescriptorErrors(t *testing.T) {
	tests := []struct {
		name        string
		removeBlob  func(map[string]ocispec.Descriptor, map[digest.Digest][]byte, ocispec.Descriptor)
		assertError func(*testing.T, error)
	}{
		{
			name: "package root manifest",
			removeBlob: func(manifests map[string]ocispec.Descriptor, blobs map[digest.Digest][]byte, _ ocispec.Descriptor) {
				delete(blobs, manifests["bundle-label"].Digest)
			},
			assertError: func(t *testing.T, err error) {
				t.Helper()
				require.ErrorIs(t, err, ErrFetchingPackageManifest)
			},
		},
		{
			name: "zarf yaml",
			removeBlob: func(_ map[string]ocispec.Descriptor, blobs map[digest.Digest][]byte, zarfDescriptor ocispec.Descriptor) {
				delete(blobs, zarfDescriptor.Digest)
			},
			assertError: func(t *testing.T, err error) {
				t.Helper()
				require.ErrorIs(t, err, ErrFetchingZarfYAML)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const packageName = "bundle-label"
			manifests, blobs, zarfDescriptor := packageMetadataFixture(t, packageName, []byte("metadata:\n  name: test\n"), true, 0)
			tt.removeBlob(manifests, blobs, zarfDescriptor)

			_, batched, err := readZarfPackageMetadataFixture(t.Context(), []string{packageName}, manifests, blobBatchFetcher{blobs: blobs})
			require.True(t, batched)
			var packageErr packageMetadataError
			require.ErrorAs(t, err, &packageErr)
			assert.Equal(t, packageName, packageErr.Package)
			tt.assertError(t, packageErr.Err)
		})
	}
}

func TestZarfPackageMetadataBatchReportsMissingManifestInPackageOrder(t *testing.T) {
	availableManifests, availableBlobs, _ := packageMetadataFixture(t, "available", []byte("metadata:\n  name: available-zarf\n"), true, 0)
	missingManifests, missingBlobs, _ := packageMetadataFixture(t, "missing", []byte("metadata:\n  name: missing-zarf\n"), true, 0)
	manifests := map[string]ocispec.Descriptor{
		"available": availableManifests["available"],
		"missing":   missingManifests["missing"],
	}
	maps.Copy(availableBlobs, missingBlobs)
	delete(availableBlobs, manifests["missing"].Digest)

	_, batched, err := readZarfPackageMetadataFixture(t.Context(), []string{"available", "missing"}, manifests, blobBatchFetcher{blobs: availableBlobs})
	require.True(t, batched)
	var packageErr packageMetadataError
	require.ErrorAs(t, err, &packageErr)
	assert.Equal(t, "missing", packageErr.Package)
	require.ErrorIs(t, packageErr.Err, ErrFetchingPackageManifest)
}

func TestZarfPackageMetadataBatchReportsEachMissingDescriptorToItsPackage(t *testing.T) {
	tests := []struct {
		name          string
		removeBlobs   func(map[string]ocispec.Descriptor, map[digest.Digest][]byte, ocispec.Descriptor, ocispec.Descriptor) ocispec.Descriptor
		expectedError error
	}{
		{
			name: "package root manifests",
			removeBlobs: func(manifests map[string]ocispec.Descriptor, blobs map[digest.Digest][]byte, _, _ ocispec.Descriptor) ocispec.Descriptor {
				delete(blobs, manifests["first"].Digest)
				delete(blobs, manifests["second"].Digest)
				return manifests["first"]
			},
			expectedError: ErrFetchingPackageManifest,
		},
		{
			name: "zarf yaml layers",
			removeBlobs: func(_ map[string]ocispec.Descriptor, blobs map[digest.Digest][]byte, first, second ocispec.Descriptor) ocispec.Descriptor {
				delete(blobs, first.Digest)
				delete(blobs, second.Digest)
				return first
			},
			expectedError: ErrFetchingZarfYAML,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firstManifests, blobs, firstZarfDescriptor := packageMetadataFixture(t, "first", []byte("metadata:\n  name: first-zarf\n"), true, 0)
			secondManifests, secondBlobs, secondZarfDescriptor := packageMetadataFixture(t, "second", []byte("metadata:\n  name: second-zarf\n"), true, 0)
			maps.Copy(blobs, secondBlobs)
			manifests := map[string]ocispec.Descriptor{
				"first":  firstManifests["first"],
				"second": secondManifests["second"],
			}
			expectedDescriptor := tt.removeBlobs(manifests, blobs, firstZarfDescriptor, secondZarfDescriptor)

			_, batched, err := readZarfPackageMetadataFixture(t.Context(), []string{"first", "second"}, manifests, blobBatchFetcher{blobs: blobs})
			require.True(t, batched)
			var packageErr packageMetadataError
			require.ErrorAs(t, err, &packageErr)
			assert.Equal(t, "first", packageErr.Package)
			require.ErrorIs(t, packageErr.Err, tt.expectedError)
			assert.ErrorContains(t, packageErr.Err, expectedDescriptor.Digest.String())
		})
	}
}

func TestZarfPackageMetadataBatchPreservesPackageOrderAheadOfLaterInvalidDescriptor(t *testing.T) {
	t.Run("package root manifest", func(t *testing.T) {
		firstManifests, blobs, _ := packageMetadataFixture(t, "first", []byte("metadata:\n  name: first-zarf\n"), true, 0)
		secondManifests, secondBlobs, _ := packageMetadataFixture(t, "second", []byte("metadata:\n  name: second-zarf\n"), true, 0)
		maps.Copy(blobs, secondBlobs)

		firstDescriptor := firstManifests["first"]
		delete(blobs, firstDescriptor.Digest)
		malformedRoot := []byte("{")
		firstDescriptor.Digest = digest.FromBytes(malformedRoot)
		firstDescriptor.Size = int64(len(malformedRoot))
		firstManifests["first"] = firstDescriptor
		blobs[firstDescriptor.Digest] = malformedRoot

		secondDescriptor := secondManifests["second"]
		secondDescriptor.Size = -1
		manifests := map[string]ocispec.Descriptor{
			"first":  firstDescriptor,
			"second": secondDescriptor,
		}

		_, batched, err := readZarfPackageMetadataFixture(t.Context(), []string{"first", "second"}, manifests, blobBatchFetcher{blobs: blobs})
		require.True(t, batched)
		var packageErr packageMetadataError
		require.ErrorAs(t, err, &packageErr)
		assert.Equal(t, "first", packageErr.Package)
		require.ErrorIs(t, packageErr.Err, ErrParsingPackageManifest)
	})

	t.Run("zarf yaml", func(t *testing.T) {
		firstManifests, blobs, _ := packageMetadataFixture(t, "first", []byte("metadata:\n  name: [\n"), true, 0)
		secondManifests, secondBlobs, secondZarfDescriptor := packageMetadataFixture(t, "second", []byte("metadata:\n  name: second-zarf\n"), true, 0)
		maps.Copy(blobs, secondBlobs)

		secondRootDescriptor := secondManifests["second"]
		var secondRoot ocispec.Manifest
		require.NoError(t, json.Unmarshal(blobs[secondRootDescriptor.Digest], &secondRoot))
		for idx, layer := range secondRoot.Layers {
			if layer.Digest == secondZarfDescriptor.Digest {
				secondRoot.Layers[idx].Size = -1
			}
		}
		secondRootBytes, err := json.Marshal(secondRoot)
		require.NoError(t, err)
		delete(blobs, secondRootDescriptor.Digest)
		secondRootDescriptor.Digest = digest.FromBytes(secondRootBytes)
		secondRootDescriptor.Size = int64(len(secondRootBytes))
		secondManifests["second"] = secondRootDescriptor
		blobs[secondRootDescriptor.Digest] = secondRootBytes

		manifests := map[string]ocispec.Descriptor{
			"first":  firstManifests["first"],
			"second": secondRootDescriptor,
		}
		_, batched, err := readZarfPackageMetadataFixture(t.Context(), []string{"first", "second"}, manifests, blobBatchFetcher{blobs: blobs})
		require.True(t, batched)
		var packageErr packageMetadataError
		require.ErrorAs(t, err, &packageErr)
		assert.Equal(t, "first", packageErr.Package)
		require.ErrorIs(t, packageErr.Err, ErrParsingZarfYAML)
	})

	t.Run("zarf yaml before invalid package root manifest", func(t *testing.T) {
		firstManifests, blobs, _ := packageMetadataFixture(t, "first", []byte("metadata:\n  name: [\n"), true, 0)
		secondManifests, secondBlobs, _ := packageMetadataFixture(t, "second", []byte("metadata:\n  name: second-zarf\n"), true, 0)
		maps.Copy(blobs, secondBlobs)

		secondDescriptor := secondManifests["second"]
		secondDescriptor.Size = -1
		manifests := map[string]ocispec.Descriptor{
			"first":  firstManifests["first"],
			"second": secondDescriptor,
		}

		_, batched, err := readZarfPackageMetadataFixture(t.Context(), []string{"first", "second"}, manifests, blobBatchFetcher{blobs: blobs})
		require.True(t, batched)
		var packageErr packageMetadataError
		require.ErrorAs(t, err, &packageErr)
		assert.Equal(t, "first", packageErr.Package)
		require.ErrorIs(t, packageErr.Err, ErrParsingZarfYAML)
	})

	t.Run("missing zarf yaml before missing package root manifest", func(t *testing.T) {
		firstManifests, blobs, firstZarfDescriptor := packageMetadataFixture(t, "first", []byte("metadata:\n  name: first-zarf\n"), true, 0)
		secondManifests, secondBlobs, _ := packageMetadataFixture(t, "second", []byte("metadata:\n  name: second-zarf\n"), true, 0)
		maps.Copy(blobs, secondBlobs)
		delete(blobs, firstZarfDescriptor.Digest)
		delete(blobs, secondManifests["second"].Digest)

		manifests := map[string]ocispec.Descriptor{
			"first":  firstManifests["first"],
			"second": secondManifests["second"],
		}
		_, batched, err := readZarfPackageMetadataFixture(t.Context(), []string{"first", "second"}, manifests, blobBatchFetcher{blobs: blobs})
		require.True(t, batched)
		var packageErr packageMetadataError
		require.ErrorAs(t, err, &packageErr)
		assert.Equal(t, "first", packageErr.Package)
		require.ErrorIs(t, packageErr.Err, ErrFetchingZarfYAML)
		assert.ErrorContains(t, packageErr.Err, firstZarfDescriptor.Digest.String())
	})
}

func TestZarfPackageMetadataBatchRetainsManifestFailureAfterVisit(t *testing.T) {
	const packageName = "bundle-label"
	manifests, blobs, _ := packageMetadataFixture(t, packageName, []byte("metadata:\n  name: test\n"), true, 0)
	fetchErr := errors.New("manifest batch failed after visit")
	fetcher := &postVisitFailingBatchFetcher{
		blobBatchFetcher: blobBatchFetcher{blobs: blobs},
		failingDigest:    manifests[packageName].Digest,
		err:              fetchErr,
	}

	_, batched, err := readZarfPackageMetadataFixture(t.Context(), []string{packageName}, manifests, fetcher)
	require.True(t, batched)
	var packageErr packageMetadataError
	require.ErrorAs(t, err, &packageErr)
	assert.Equal(t, packageName, packageErr.Package)
	require.ErrorIs(t, packageErr.Err, ErrFetchingPackageManifest)
	require.ErrorIs(t, packageErr.Err, fetchErr)
}

func TestZarfPackageMetadataBatchReportsDescriptorFailureForOwningPackage(t *testing.T) {
	firstManifests, blobs, _ := packageMetadataFixture(t, "first", []byte("metadata:\n  name: first-zarf\n"), true, 0)
	secondManifests, secondBlobs, secondZarfDescriptor := packageMetadataFixture(t, "second", []byte("metadata:\n  name: second-zarf\n"), true, 0)
	maps.Copy(blobs, secondBlobs)
	manifests := map[string]ocispec.Descriptor{
		"first":  firstManifests["first"],
		"second": secondManifests["second"],
	}

	tests := []struct {
		name          string
		descriptor    ocispec.Descriptor
		expectedError error
	}{
		{
			name:          "package root manifest",
			descriptor:    manifests["second"],
			expectedError: ErrFetchingPackageManifest,
		},
		{
			name:          "zarf yaml",
			descriptor:    secondZarfDescriptor,
			expectedError: ErrFetchingZarfYAML,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetchErr := errors.New("descriptor read failed")
			fetcher := descriptorFailingBatchFetcher{
				blobBatchFetcher: blobBatchFetcher{blobs: blobs},
				failingDigest:    tt.descriptor.Digest,
				err:              fetchErr,
			}

			_, batched, err := readZarfPackageMetadataFixture(t.Context(), []string{"first", "second"}, manifests, fetcher)
			require.True(t, batched)
			var packageErr packageMetadataError
			require.ErrorAs(t, err, &packageErr)
			assert.Equal(t, "second", packageErr.Package)
			require.ErrorIs(t, packageErr.Err, tt.expectedError)
			require.ErrorIs(t, packageErr.Err, fetchErr)
		})
	}
}

func TestReadBundleZarfMetadataBatchPreservesOrderAcrossIndexLookup(t *testing.T) {
	firstManifests, blobs, _ := packageMetadataFixture(t, "first", []byte("metadata:\n  name: [\n"), true, 0)
	firstManifest := firstManifests["first"]
	firstManifest.Annotations = map[string]string{udsoci.AnnotationPackageName: "first"}
	idx := ocispec.Index{Manifests: []ocispec.Descriptor{firstManifest}}
	bundle := &spec.UDSBundle{Packages: []spec.Package{{Name: "first"}, {Name: "missing"}}}

	_, batched, err := readBundleZarfMetadataBatch(t.Context(), idx, bundle, nil, blobBatchFetcher{blobs: blobs})
	require.True(t, batched)
	var packageErr packageMetadataError
	require.ErrorAs(t, err, &packageErr)
	assert.Equal(t, "first", packageErr.Package)
	require.ErrorIs(t, packageErr.Err, ErrParsingZarfYAML)
}

func TestReadPackageSignaturesDoesNotRetryBatchWideFailure(t *testing.T) {
	const packageName = "bundle-label"
	batchErr := errors.New("archive unavailable")
	fetcher := &failingBatchFetcher{err: batchErr}
	manifest := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString("manifest"),
		Size:      1,
		Annotations: map[string]string{
			udsoci.AnnotationPackageName: packageName,
		},
	}
	indexBytes, err := json.Marshal(ocispec.Index{Manifests: []ocispec.Descriptor{manifest}})
	require.NoError(t, err)
	source := &MetadataSource{IndexBytes: indexBytes, Fetcher: fetcher}
	bundle := &spec.UDSBundle{Packages: []spec.Package{{Name: packageName}}}

	_, err = ReadPackageSignatures(t.Context(), source, bundle)
	require.ErrorIs(t, err, batchErr)
	var target InspectingPackageSignatureError
	require.ErrorAs(t, err, &target)
	assert.Equal(t, packageName, target.Package)
	assert.Equal(t, 1, fetcher.batchReads)
	assert.Zero(t, fetcher.fetches)
}

func TestFetchZarfPackageClassifiesMalformedYAML(t *testing.T) {
	manifests, blobs, _ := packageMetadataFixture(t, "bundle-label", []byte("metadata:\n  name: [\n"), true, 0)

	_, _, err := fetchZarfPackage(t.Context(), "bundle-label", manifests["bundle-label"], blobFetcher(blobs, nil))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrParsingZarfYAML)
	require.NotErrorIs(t, err, ErrFetchingZarfYAML)
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
	require.ErrorIs(t, err, ErrFetchingZarfYAML)
	require.ErrorIs(t, err, fetchErr)
	require.NotErrorIs(t, err, ErrParsingZarfYAML)
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
	require.ErrorIs(t, err, ErrFetchingZarfYAML)
	require.Equal(t, 0, zarfReads)
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
	require.Equal(t, "migrated-package", pkg.Metadata.Name)
	require.Len(t, pkg.Components, 1)
	require.Len(t, pkg.Components[0].Actions.OnDeploy.Before, 1)
	require.Equal(t, "echo migrated", pkg.Components[0].Actions.OnDeploy.Before[0].Cmd)
}

func TestInspectPackageSignatureUsesZarfMetadata(t *testing.T) {
	t.Run("signed package", func(t *testing.T) {
		manifests, blobs, _ := packageMetadataFixture(t, "bundle-label", []byte("metadata:\n  name: test\nbuild:\n  signed: true\n"), true, 0)
		entry := manifests["bundle-label"]
		entry.Annotations = map[string]string{udsoci.AnnotationPackageName: "bundle-label"}
		idx := ocispec.Index{Manifests: []ocispec.Descriptor{entry}}

		summary, err := inspectPackageSignature(t.Context(), idx, spec.Package{Name: "bundle-label"}, blobFetcher(blobs, nil))
		require.NoError(t, err)
		require.Equal(t, PackageSigningStatusSigned, summary.Signed)
	})

	t.Run("missing zarf yaml", func(t *testing.T) {
		manifests, blobs, _ := packageMetadataFixture(t, "bundle-label", nil, false, 0)
		entry := manifests["bundle-label"]
		entry.Annotations = map[string]string{udsoci.AnnotationPackageName: "bundle-label"}
		idx := ocispec.Index{Manifests: []ocispec.Descriptor{entry}}

		summary, err := inspectPackageSignature(t.Context(), idx, spec.Package{Name: "bundle-label"}, blobFetcher(blobs, nil))
		require.NoError(t, err)
		require.Equal(t, PackageSigningStatusUnknown, summary.Signed)
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
