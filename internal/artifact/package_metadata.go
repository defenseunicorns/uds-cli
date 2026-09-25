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
	FetchBatch(context.Context, []ocispec.Descriptor, func(ocispec.Descriptor, []byte, error) error) error
}

// zarfPackageMetadata contains the final metadata extracted from a package's zarf.yaml.
type zarfPackageMetadata struct {
	zarfName string
	signed   *bool
	found    bool
}

// bundlePackageMetadataState tracks one bundle package through batched metadata parsing.
type bundlePackageMetadataState struct {
	packageName string
	manifest    *ocispec.Descriptor
	root        *oci.Manifest
	result      zarfPackageMetadata
	err         error
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

// readZarfPackageMetadataBatch parses zarf manifests in a batch to minimize artifact decompression.
// The boolean is false only when batching is unavailable and the caller should use the ordinary fetch path.
func readZarfPackageMetadataBatch(ctx context.Context, states []bundlePackageMetadataState, fetcher content.Fetcher) (map[string]zarfPackageMetadata, bool, error) {
	batchFetcher, ok := fetcher.(descriptorBatchFetcher)
	if !ok {
		return nil, false, nil
	}

	manifestStates := make(map[digest.Digest][]*bundlePackageMetadataState, len(states))
	manifestDescriptors := make([]ocispec.Descriptor, 0, len(states))
	for idx := range states {
		state := &states[idx]
		if state.manifest == nil || state.err != nil {
			continue
		}
		descriptor := *state.manifest
		if err := validateMetadataDescriptor(descriptor); err != nil {
			state.err = wrapBatchPackageManifestFetchError(state.packageName, descriptor, err)
			continue
		}
		if len(manifestStates[descriptor.Digest]) == 0 {
			manifestDescriptors = append(manifestDescriptors, descriptor)
		}
		manifestStates[descriptor.Digest] = append(manifestStates[descriptor.Digest], state)
	}

	if len(manifestDescriptors) > 0 {
		err := batchFetcher.FetchBatch(ctx, manifestDescriptors, func(descriptor ocispec.Descriptor, data []byte, fetchErr error) error {
			for _, state := range manifestStates[descriptor.Digest] {
				entry := *state.manifest
				if fetchErr != nil {
					state.err = wrapBatchPackageManifestFetchError(state.packageName, entry, fetchErr)
					continue
				}
				root, err := fetchPackageRootManifest(ctx, state.packageName, entry, descriptorBytesFetcher(entry, data))
				if err != nil {
					state.err = err
					continue
				}
				zarfLayer := root.Locate(zarflayout.ZarfYAML)
				if oci.IsEmptyDescriptor(zarfLayer) {
					continue
				}
				root.Manifest = ocispec.Manifest{Layers: []ocispec.Descriptor{zarfLayer}}
				state.root = &root
			}
			return nil
		})
		if err != nil {
			state := manifestStates[manifestDescriptors[0].Digest][0]
			state.err = wrapBatchPackageManifestFetchError(state.packageName, *state.manifest, err)
			return finishZarfPackageMetadataBatch(states, true)
		}
	}

	zarfStates := make(map[digest.Digest][]*bundlePackageMetadataState, len(states))
	zarfDescriptors := make([]ocispec.Descriptor, 0, len(states))
	for idx := range states {
		state := &states[idx]
		if state.root == nil || state.err != nil {
			continue
		}
		descriptor := state.root.Locate(zarflayout.ZarfYAML)
		if err := validateMetadataDescriptor(descriptor); err != nil {
			state.result.found = true
			state.err = wrapZarfYAMLFetchError(state.packageName, descriptor, err)
			continue
		}
		if len(zarfStates[descriptor.Digest]) == 0 {
			zarfDescriptors = append(zarfDescriptors, descriptor)
		}
		zarfStates[descriptor.Digest] = append(zarfStates[descriptor.Digest], state)
	}

	if len(zarfDescriptors) > 0 {
		err := batchFetcher.FetchBatch(ctx, zarfDescriptors, func(descriptor ocispec.Descriptor, data []byte, fetchErr error) error {
			for _, state := range zarfStates[descriptor.Digest] {
				if fetchErr != nil {
					state.result.found = true
					state.err = wrapZarfYAMLFetchError(state.packageName, descriptor, fetchErr)
					continue
				}
				pkg, found, err := fetchZarfPackageFromManifest(ctx, state.packageName, *state.root, descriptorBytesFetcher(descriptor, data))
				metadata := zarfPackageMetadata{found: found}
				state.err = err
				if err == nil && found {
					metadata.zarfName = pkg.Metadata.Name
					if pkg.Build.Signed != nil {
						signed := *pkg.Build.Signed
						metadata.signed = &signed
					}
				}
				state.result = metadata
			}
			return nil
		})
		if err != nil {
			state := zarfStates[zarfDescriptors[0].Digest][0]
			descriptor := state.root.Locate(zarflayout.ZarfYAML)
			state.result.found = true
			state.err = wrapZarfYAMLFetchError(state.packageName, descriptor, err)
		}
	}

	return finishZarfPackageMetadataBatch(states, true)
}

func finishZarfPackageMetadataBatch(states []bundlePackageMetadataState, batched bool) (map[string]zarfPackageMetadata, bool, error) {
	results := make(map[string]zarfPackageMetadata, len(states))
	for _, state := range states {
		if state.err != nil {
			return nil, batched, packageMetadataError{Package: state.packageName, Err: state.err}
		}
		results[state.packageName] = state.result
	}
	return results, batched, nil
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
		return oci.Manifest{}, wrapPackageManifestFetchError(packageName, entry, err)
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

func wrapPackageManifestFetchError(packageName string, entry ocispec.Descriptor, err error) error {
	return fmt.Errorf("%w %s for package %q: %w", ErrFetchingPackageManifest, entry.Digest, packageName, err)
}

func wrapBatchPackageManifestFetchError(packageName string, entry ocispec.Descriptor, err error) error {
	fetchErr := fmt.Errorf("fetching %s: %w: %w", entry.Digest, udsoci.ErrFetchContent, err)
	return wrapPackageManifestFetchError(packageName, entry, fetchErr)
}

func wrapZarfYAMLFetchError(packageName string, descriptor ocispec.Descriptor, err error) error {
	layerErr := fmt.Errorf("%w: %w", ErrFetchingZarfLayer, err)
	return fmt.Errorf("%w %s for package %q: %w", ErrFetchingZarfYAML, descriptor.Digest, packageName, layerErr)
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
