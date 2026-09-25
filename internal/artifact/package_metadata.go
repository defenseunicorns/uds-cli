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
	"sort"

	"github.com/defenseunicorns/pkg/oci"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zarflayout "github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	"oras.land/oras-go/v2/content"
)

// descriptorBatchFetcher is an optional capability implemented by fetchers
// that can retrieve several descriptor bodies without repeating source setup.
type descriptorBatchFetcher interface {
	FetchBatch(context.Context, []ocispec.Descriptor, func(ocispec.Descriptor, []byte) error) error
}

type zarfPackageMetadata struct {
	name   string
	signed *bool
	found  bool
	err    error
}

type zarfLayerReader struct{ io.ReadCloser }

func (r zarfLayerReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, fmt.Errorf("%w: %w", ErrFetchingZarfLayer, err)
	}
	return n, err
}

func readPackageZarfNames(ctx context.Context, manifests map[string]ocispec.Descriptor, fetcher content.Fetcher) (map[string]string, error) {
	packageNames := make([]string, 0, len(manifests))
	for packageName := range manifests {
		packageNames = append(packageNames, packageName)
	}
	sort.Strings(packageNames)
	zarfNames := make(map[string]string, len(manifests))
	for _, packageName := range packageNames {
		pkg, found, err := fetchZarfPackage(ctx, packageName, manifests[packageName], fetcher)
		if err != nil {
			return nil, err
		}
		if !found || pkg.Metadata.Name == "" {
			return nil, MissingZarfPackageNameError{Package: packageName}
		}
		zarfNames[packageName] = pkg.Metadata.Name
	}
	return zarfNames, nil
}

// readZarfPackageMetadataBatch parses zarf manifests in a batch, to minimize occurrences artifact decompression.
//
// Package-specific errors are retained in the result map so callers can report them in bundle order.
// The boolean is false when batching is unavailable or fails and the caller should use the ordinary fetch path.
func readZarfPackageMetadataBatch(ctx context.Context, packageNames []string, manifests map[string]ocispec.Descriptor, fetcher content.Fetcher) (map[string]zarfPackageMetadata, bool) {
	batchFetcher, ok := fetcher.(descriptorBatchFetcher)
	if !ok {
		return nil, false
	}

	results := make(map[string]zarfPackageMetadata, len(packageNames))
	if len(packageNames) == 0 {
		return results, true
	}
	// Group package manifests by digest so shared content is fetched only once.
	roots := make(map[string]oci.Manifest, len(packageNames))
	manifestPackages := make(map[digest.Digest][]string, len(packageNames))
	manifestDescriptors := make([]ocispec.Descriptor, 0, len(packageNames))
	for _, packageName := range packageNames {
		descriptor := manifests[packageName]
		if len(manifestPackages[descriptor.Digest]) == 0 {
			manifestDescriptors = append(manifestDescriptors, descriptor)
		}
		manifestPackages[descriptor.Digest] = append(manifestPackages[descriptor.Digest], packageName)
	}
	// verify and parse every package root manifest, to later locate its zarf.yaml layer.
	if err := batchFetcher.FetchBatch(ctx, manifestDescriptors, func(descriptor ocispec.Descriptor, data []byte) error {
		for _, packageName := range manifestPackages[descriptor.Digest] {
			entry := manifests[packageName]
			root, err := fetchPackageRootManifest(ctx, packageName, entry, descriptorBytesFetcher(entry, data))
			if err != nil {
				results[packageName] = zarfPackageMetadata{err: err}
				continue
			}
			zarfLayer := root.Locate(zarflayout.ZarfYAML)
			if oci.IsEmptyDescriptor(zarfLayer) {
				results[packageName] = zarfPackageMetadata{}
				continue
			}
			root.Manifest = ocispec.Manifest{Layers: []ocispec.Descriptor{zarfLayer}}
			roots[packageName] = root
		}
		return nil
	}); err != nil {
		return nil, false
	}

	// Group the discovered zarf.yaml layers by digest as multiple package manifests may reference the same layer.
	zarfPackages := make(map[digest.Digest][]string, len(roots))
	zarfDescriptors := make([]ocispec.Descriptor, 0, len(roots))
	for packageName, root := range roots {
		descriptor := root.Locate(zarflayout.ZarfYAML)
		if len(zarfPackages[descriptor.Digest]) == 0 {
			zarfDescriptors = append(zarfDescriptors, descriptor)
		}
		zarfPackages[descriptor.Digest] = append(zarfPackages[descriptor.Digest], packageName)
	}
	if len(zarfDescriptors) == 0 {
		return results, true
	}
	// parse each zarf.yaml through the Zarf parser
	if err := batchFetcher.FetchBatch(ctx, zarfDescriptors, func(descriptor ocispec.Descriptor, data []byte) error {
		for _, packageName := range zarfPackages[descriptor.Digest] {
			pkg, found, err := fetchZarfPackageFromManifest(ctx, packageName, roots[packageName], descriptorBytesFetcher(descriptor, data))
			metadata := zarfPackageMetadata{found: found, err: err}
			if err == nil && found {
				metadata.name = pkg.Metadata.Name
				if pkg.Build.Signed != nil {
					signed := *pkg.Build.Signed
					metadata.signed = &signed
				}
			}
			results[packageName] = metadata
		}
		return nil
	}); err != nil {
		return nil, false
	}
	return results, true
}

// fetchZarfPackage fetches and parses the embedded zarf.yaml into a ZarfPackage.
// The boolean reports whether the package manifest contained a zarf.yaml layer.
func fetchZarfPackage(ctx context.Context, packageName string, entry ocispec.Descriptor, fetcher content.Fetcher) (v1alpha1.ZarfPackage, bool, error) {
	root, err := fetchPackageRootManifest(ctx, packageName, entry, fetcher)
	if err != nil {
		return v1alpha1.ZarfPackage{}, false, err
	}
	return fetchZarfPackageFromManifest(ctx, packageName, root, fetcher)
}

func fetchPackageRootManifest(ctx context.Context, packageName string, entry ocispec.Descriptor, fetcher content.Fetcher) (oci.Manifest, error) {
	manifestBytes, err := udsoci.FetchBytes(ctx, fetcher, entry)
	if err != nil {
		return oci.Manifest{}, fmt.Errorf("%w %s for package %q: %w", ErrFetchingPackageManifest, entry.Digest, packageName, err)
	}
	var root oci.Manifest
	if err := json.Unmarshal(manifestBytes, &root); err != nil {
		return oci.Manifest{}, fmt.Errorf("%w %s for package %q: %w", ErrParsingPackageManifest, entry.Digest, packageName, err)
	}
	if root.SchemaVersion != 2 {
		return oci.Manifest{}, UnsupportedSchemaVersionError{Artifact: "package manifest", Version: root.SchemaVersion}
	}
	if root.MediaType != "" && !udsoci.IsImageManifestMediaType(root.MediaType) {
		return oci.Manifest{}, UnsupportedMediaTypeError{Artifact: "package manifest", MediaType: root.MediaType}
	}
	return root, nil
}

func fetchZarfPackageFromManifest(ctx context.Context, packageName string, root oci.Manifest, fetcher content.Fetcher) (v1alpha1.ZarfPackage, bool, error) {
	zarfLayer := root.Locate(zarflayout.ZarfYAML)
	if oci.IsEmptyDescriptor(zarfLayer) {
		return v1alpha1.ZarfPackage{}, false, nil
	}

	// FetchZarfYAML uses content.FetchAll internally. Reject oversized metadata
	// before opening it, then let ORAS stream, bound, and verify the descriptor.
	boundedFetcher := content.FetcherFunc(func(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
		if err := desc.Digest.Validate(); err != nil {
			return nil, fmt.Errorf("%w: invalid digest %s: %w", ErrFetchingZarfLayer, desc.Digest, err)
		}
		if desc.Size < 0 {
			return nil, fmt.Errorf("%w: %w", ErrFetchingZarfLayer, content.ErrInvalidDescriptorSize)
		}
		if desc.Size > udsoci.MaxFetchBytesSize {
			err := udsoci.DescriptorTooLargeError{Digest: desc.Digest, Size: desc.Size, Limit: udsoci.MaxFetchBytesSize}
			return nil, fmt.Errorf("%w: %w", ErrFetchingZarfLayer, err)
		}
		r, err := fetcher.Fetch(ctx, desc)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrFetchingZarfLayer, err)
		}
		return zarfLayerReader{ReadCloser: r}, nil
	})
	pkg, err := zoci.FetchZarfYAML(ctx, &root, boundedFetcher)
	if err != nil {
		if isZarfLayerReadError(err) {
			return v1alpha1.ZarfPackage{}, true, fmt.Errorf("%w %s for package %q: %w", ErrFetchingZarfYAML, zarfLayer.Digest, packageName, err)
		}
		return v1alpha1.ZarfPackage{}, true, fmt.Errorf("%w %s for package %q: %w", ErrParsingZarfYAML, zarfLayer.Digest, packageName, err)
	}
	return pkg, true, nil
}

func descriptorBytesFetcher(descriptor ocispec.Descriptor, data []byte) content.Fetcher {
	return content.FetcherFunc(func(_ context.Context, requested ocispec.Descriptor) (io.ReadCloser, error) {
		if requested.Digest != descriptor.Digest {
			return nil, fmt.Errorf("blob %s not found", requested.Digest)
		}
		return io.NopCloser(bytes.NewReader(data)), nil
	})
}

func isZarfLayerReadError(err error) bool {
	return errors.Is(err, ErrFetchingZarfLayer) ||
		errors.Is(err, content.ErrInvalidDescriptorSize) ||
		errors.Is(err, content.ErrMismatchedDigest) ||
		errors.Is(err, content.ErrTrailingData) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}
