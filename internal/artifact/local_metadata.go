// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
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

func (f archiveContentFetcher) Fetch(ctx context.Context, descriptor ocispec.Descriptor) (io.ReadCloser, error) {
	if err := descriptor.Digest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid descriptor digest %s: %w", descriptor.Digest, err)
	}
	if descriptor.Size < 0 {
		return nil, content.ErrInvalidDescriptorSize
	}
	if descriptor.Size > udsoci.MaxFetchBytesSize {
		return nil, udsoci.DescriptorTooLargeError{
			Digest: descriptor.Digest,
			Size:   descriptor.Size,
			Limit:  udsoci.MaxFetchBytesSize,
		}
	}
	entryPath := path.Join("oci", "blobs", descriptor.Digest.Algorithm().String(), descriptor.Digest.Encoded())
	entries, err := readTarZstEntries(ctx, f.source, map[string]struct{}{entryPath: {}})
	if err != nil {
		return nil, err
	}
	data, ok := entries[entryPath]
	if !ok {
		return nil, fmt.Errorf("blob %s not found in local archive", descriptor.Digest)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
