// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"io"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	orasregistry "oras.land/oras-go/v2/registry/remote"
)

// ParseDigest parses and validates an OCI digest string.
func ParseDigest(value string) (digest.Digest, error) {
	return parseDigest(value)
}

// SHA256Digest constructs a SHA-256 digest from its encoded value.
func SHA256Digest(encoded string) digest.Digest {
	return sha256Digest(encoded)
}

// WriteBlobBytesIfMissingAndVerify verifies and stores blob bytes when absent.
func WriteBlobBytesIfMissingAndVerify(blobDir string, d digest.Digest, data []byte) error {
	return writeBlobBytesIfMissingAndVerify(blobDir, d, data)
}

// WriteBlobReaderIfMissingAndVerify verifies and stores streamed blob content when absent.
func WriteBlobReaderIfMissingAndVerify(blobDir string, d digest.Digest, reader io.Reader) error {
	return writeBlobReaderIfMissingAndVerify(blobDir, d, reader)
}

// CopyBlobFileIfMissingAndVerify copies a blob unless verified content already exists.
func CopyBlobFileIfMissingAndVerify(dstBlobDir, srcPath string, d digest.Digest) error {
	return copyBlobFileIfMissingAndVerify(dstBlobDir, srcPath, d)
}

// CopyRequiredBlobsFromLayout copies manifests and referenced content between layouts.
func CopyRequiredBlobsFromLayout(dstBlobDir, srcBlobDir string, manifests []OciManifest) error {
	return copyRequiredBlobsFromLayout(dstBlobDir, srcBlobDir, manifests)
}

// WriteOCILayout writes an OCI layout marker file.
func WriteOCILayout(path string) error {
	return writeOCILayout(path)
}

// WriteOCIIndex writes an OCI image index.
func WriteOCIIndex(path string, idx *OciIndex) error {
	return writeOCIIndex(path, idx)
}

// VerifyOCILayoutDigests verifies all digests referenced by an OCI layout.
func VerifyOCILayoutDigests(ctx context.Context, streams iostreams.IOStreams, ociDir string) error {
	return verifyOCILayoutDigests(ctx, streams, ociDir)
}

// GCUnreferencedBlobs removes blobs not referenced by the supplied manifests.
func GCUnreferencedBlobs(ctx context.Context, streams iostreams.IOStreams, blobDir string, manifests []OciManifest) error {
	return gcUnreferencedBlobs(ctx, streams, blobDir, manifests)
}

// FindBundleDefinitionEntry returns the bundle definition manifest and its index.
func FindBundleDefinitionEntry(idx OciIndex) (*OciManifest, int, error) {
	return findBundleDefinitionEntry(idx)
}

// IsBundleIndex reports whether idx is a canonical bundle index.
func IsBundleIndex(idx OciIndex) bool {
	return isBundleIndex(idx)
}

// NewBundleIndex creates a deterministic single-architecture bundle index.
func NewBundleIndex(manifests []OciManifest, arch string) *OciIndex {
	return newBundleIndex(manifests, arch)
}

// WriteAndDigestBlob writes data into an OCI blob directory and returns its digest.
func WriteAndDigestBlob(blobDir string, data []byte) (digest.Digest, error) {
	return writeAndDigestBlob(blobDir, data)
}

// FindLayerByTitle returns a manifest layer with the requested OCI title.
func FindLayerByTitle(manifest OciImageManifest, title string) (OciDescriptor, error) {
	return findLayerByTitle(manifest, title)
}

// SortManifestsByDigest sorts manifest descriptors deterministically.
func SortManifestsByDigest(manifests []OciManifest) {
	sortManifestsByDigest(manifests)
}

// NewRemoteRepository creates an authenticated remote OCI repository.
func NewRemoteRepository(ctx context.Context, ref string, opts ConfigOptions) (*orasregistry.Repository, error) {
	return newRemoteRepository(ctx, ref, opts)
}

// ResolveBundleChild resolves a bundle's architecture-specific child index.
func ResolveBundleChild(ctx context.Context, src oras.Target, reference, arch string) (ocispec.Descriptor, []byte, error) {
	return resolveBundleChild(ctx, src, reference, arch)
}

// MergeRootIndex merges a child descriptor into a bundle root index.
func MergeRootIndex(ctx context.Context, dst oras.Target, tag string, child ocispec.Descriptor) ([]byte, ocispec.Descriptor, error) {
	return mergeRootIndex(ctx, dst, tag, child)
}
