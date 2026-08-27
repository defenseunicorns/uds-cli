// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/defenseunicorns/pkg/oci"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zarflayout "github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	"oras.land/oras-go/v2/content"
)

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

// fetchZarfPackage fetches and parses the embedded zarf.yaml into a ZarfPackage.
// The boolean reports whether the package manifest contained a zarf.yaml layer.
func fetchZarfPackage(ctx context.Context, packageName string, entry ocispec.Descriptor, fetcher content.Fetcher) (v1alpha1.ZarfPackage, bool, error) {
	manifestBytes, err := udsoci.FetchBytes(ctx, fetcher, entry)
	if err != nil {
		return v1alpha1.ZarfPackage{}, false, fmt.Errorf("%w %s for package %q: %w", ErrFetchingPackageManifest, entry.Digest, packageName, err)
	}
	var root oci.Manifest
	if err := json.Unmarshal(manifestBytes, &root); err != nil {
		return v1alpha1.ZarfPackage{}, false, fmt.Errorf("%w %s for package %q: %w", ErrParsingPackageManifest, entry.Digest, packageName, err)
	}
	if root.SchemaVersion != 2 {
		return v1alpha1.ZarfPackage{}, false, UnsupportedSchemaVersionError{Artifact: "package manifest", Version: root.SchemaVersion}
	}
	if root.MediaType != "" && !udsoci.IsImageManifestMediaType(root.MediaType) {
		return v1alpha1.ZarfPackage{}, false, UnsupportedMediaTypeError{Artifact: "package manifest", MediaType: root.MediaType}
	}

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

func isZarfLayerReadError(err error) bool {
	return errors.Is(err, ErrFetchingZarfLayer) ||
		errors.Is(err, content.ErrInvalidDescriptorSize) ||
		errors.Is(err, content.ErrMismatchedDigest) ||
		errors.Is(err, content.ErrTrailingData) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}
