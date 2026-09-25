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

// Fetch returns one OCI blob from the archive
func (f archiveContentFetcher) Fetch(ctx context.Context, descriptor ocispec.Descriptor) (io.ReadCloser, error) {
	var data []byte
	err := f.FetchBatch(ctx, []ocispec.Descriptor{descriptor}, func(_ ocispec.Descriptor, fetched []byte) error {
		data = fetched
		return nil
	})
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// FetchBatch walks the compressed archive once and invokes visit for each
// requested descriptor it finds. The callback receives the complete raw OCI
// blob bytes; it must verify them against the descriptor before parsing them.
// Batch callers should process each body immediately to keep memory bounded.
func (f archiveContentFetcher) FetchBatch(ctx context.Context, descriptors []ocispec.Descriptor, visit func(ocispec.Descriptor, []byte) error) error {
	// Index requested descriptors by their expected OCI archive paths so one
	// archive walk can ignore all unrelated entries.
	byPath := make(map[string]ocispec.Descriptor, len(descriptors))
	for _, descriptor := range descriptors {
		if err := validateMetadataDescriptor(descriptor); err != nil {
			return err
		}
		entryPath := path.Join("oci", "blobs", descriptor.Digest.Algorithm().String(), descriptor.Digest.Encoded())
		byPath[entryPath] = descriptor
	}
	// Stream the archive once and process each requested blob as it appears.
	found := make(map[digest.Digest]struct{}, len(byPath))
	if err := walkTarZst(ctx, f.source, func(header *tar.Header, reader io.Reader) error {
		entryPath := path.Clean(header.Name)
		descriptor, ok := byPath[entryPath]
		if !ok {
			return nil
		}
		if _, duplicate := found[descriptor.Digest]; duplicate {
			return fmt.Errorf("archive contains duplicate entry %q", entryPath)
		}
		if header.Size < 0 || header.Size > udsoci.MaxFetchBytesSize {
			return fmt.Errorf("archive entry %q is %d bytes, larger than the %d byte buffered read limit", entryPath, header.Size, udsoci.MaxFetchBytesSize)
		}
		// Buffer the current metadata blob
		data, err := io.ReadAll(io.LimitReader(reader, udsoci.MaxFetchBytesSize+1))
		if err != nil {
			return fmt.Errorf("reading archive entry %q: %w", entryPath, err)
		}
		if int64(len(data)) != header.Size {
			return fmt.Errorf("reading archive entry %q: expected %d bytes, got %d", entryPath, header.Size, len(data))
		}
		if err := visit(descriptor, data); err != nil {
			return err
		}
		found[descriptor.Digest] = struct{}{}
		return nil
	}); err != nil {
		return err
	}
	// Verify requested digests were passed to the callback
	for _, descriptor := range byPath {
		if _, ok := found[descriptor.Digest]; !ok {
			return fmt.Errorf("blob %s not found in local archive", descriptor.Digest)
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
