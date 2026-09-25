// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"

	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
)

// LocalArchiveMetadataSource exposes a bundle archive as OCI metadata without
// extracting its layout. Fetcher reads individual descriptor blobs on demand.
type LocalArchiveMetadataSource struct {
	Index             []byte
	ArtifactDigest    string
	Fetcher           content.Fetcher
	SignatureEvidence []byte
	SignatureFound    bool
}

// OpenLocalArchiveMetadataSource opens the index and signature evidence from a
// local .tar.zst and returns an OCI fetcher backed directly by the archive.
func OpenLocalArchiveMetadataSource(ctx context.Context, source string) (*LocalArchiveMetadataSource, error) {
	initial, err := readTarZstEntries(ctx, source, map[string]struct{}{
		"oci/index.json":               {},
		udsoci.BundleSignatureFileName: {},
	})
	if err != nil {
		return nil, err
	}
	indexBytes, ok := initial["oci/index.json"]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrReadingBundleIndex, "oci/index.json")
	}
	evidence, signatureFound := initial[udsoci.BundleSignatureFileName]
	return &LocalArchiveMetadataSource{
		Index:             indexBytes,
		ArtifactDigest:    digest.FromBytes(indexBytes).String(),
		Fetcher:           archiveContentFetcher{source: source},
		SignatureEvidence: evidence,
		SignatureFound:    signatureFound,
	}, nil
}

type archiveContentFetcher struct{ source string }

// Fetch returns one OCI blob from the archive.
func (f archiveContentFetcher) Fetch(ctx context.Context, descriptor ocispec.Descriptor) (io.ReadCloser, error) {
	var data []byte
	var fetchErr error
	err := f.FetchBatch(ctx, []ocispec.Descriptor{descriptor}, func(_ ocispec.Descriptor, fetched []byte, descriptorErr error) error {
		data = fetched
		fetchErr = descriptorErr
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fetchErr != nil {
		return nil, fetchErr
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// FetchBatch walks the compressed archive once and reports descriptor content
// or descriptor-specific failures to visit. Its return value is reserved for
// archive-wide failures and callback errors. Batch callers should process each
// body immediately to keep memory bounded.
func (f archiveContentFetcher) FetchBatch(ctx context.Context, descriptors []ocispec.Descriptor, visit func(ocispec.Descriptor, []byte, error) error) error {
	byPath := make(map[string]ocispec.Descriptor, len(descriptors))
	reported := make(map[digest.Digest]struct{}, len(descriptors))
	for _, descriptor := range descriptors {
		if err := validateMetadataDescriptor(descriptor); err != nil {
			reported[descriptor.Digest] = struct{}{}
			if visitErr := visit(descriptor, nil, err); visitErr != nil {
				return visitErr
			}
			continue
		}
		entryPath := path.Join("oci", "blobs", descriptor.Digest.Algorithm().String(), descriptor.Digest.Encoded())
		byPath[entryPath] = descriptor
	}
	if len(byPath) == 0 {
		return nil
	}

	if err := walkTarZst(ctx, f.source, func(header *tar.Header, reader io.Reader) error {
		entryPath := path.Clean(header.Name)
		descriptor, ok := byPath[entryPath]
		if !ok {
			return nil
		}
		if _, duplicate := reported[descriptor.Digest]; duplicate {
			err := fmt.Errorf("archive contains duplicate entry %q", entryPath)
			return visit(descriptor, nil, err)
		}
		reported[descriptor.Digest] = struct{}{}
		if header.Size < 0 || header.Size > udsoci.MaxFetchBytesSize {
			err := fmt.Errorf("archive entry %q is %d bytes, larger than the %d byte buffered read limit", entryPath, header.Size, udsoci.MaxFetchBytesSize)
			return visit(descriptor, nil, err)
		}
		data, err := io.ReadAll(io.LimitReader(reader, udsoci.MaxFetchBytesSize+1))
		if err != nil {
			err = fmt.Errorf("reading archive entry %q: %w", entryPath, err)
			return visit(descriptor, nil, err)
		}
		if int64(len(data)) != header.Size {
			err := fmt.Errorf("reading archive entry %q: expected %d bytes, got %d", entryPath, header.Size, len(data))
			return visit(descriptor, nil, err)
		}
		return visit(descriptor, data, nil)
	}); err != nil {
		return err
	}

	for _, descriptor := range byPath {
		if _, ok := reported[descriptor.Digest]; ok {
			continue
		}
		err := fmt.Errorf("blob %s not found in local archive", descriptor.Digest)
		if visitErr := visit(descriptor, nil, err); visitErr != nil {
			return visitErr
		}
	}
	return nil
}

func validateMetadataDescriptor(descriptor ocispec.Descriptor) error {
	if err := descriptor.Digest.Validate(); err != nil {
		return fmt.Errorf("invalid descriptor digest %s: %w", descriptor.Digest, err)
	}
	if descriptor.Size < 0 {
		return content.ErrInvalidDescriptorSize
	}
	if descriptor.Size > udsoci.MaxFetchBytesSize {
		return udsoci.DescriptorTooLargeError{Digest: descriptor.Digest, Size: descriptor.Size, Limit: udsoci.MaxFetchBytesSize}
	}
	return nil
}
