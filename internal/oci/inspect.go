// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const mediaTypeDockerManifest = "application/vnd.docker.distribution.manifest.v2+json"

// IsImageManifestMediaType reports whether mediaType identifies an OCI or Docker image manifest.
func IsImageManifestMediaType(mediaType string) bool {
	return mediaType == ocispec.MediaTypeImageManifest || mediaType == mediaTypeDockerManifest
}

// FindOCILayoutRoot finds an OCI image layout at the root or a conventional child directory.
func FindOCILayoutRoot(root string) (string, error) {
	for _, candidate := range []string{root, filepath.Join(root, "oci"), filepath.Join(root, "images")} {
		if isOCILayoutDir(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no OCI image layout found in %q", root)
}

func isOCILayoutDir(dir string) bool {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return false
	}
	for _, path := range []string{
		filepath.Join(dir, "oci-layout"),
		filepath.Join(dir, "index.json"),
		filepath.Join(dir, "blobs", "sha256"),
	} {
		if _, err := os.Stat(path); err != nil {
			return false
		}
	}
	return true
}

// DescriptorFromOCI converts an internal OCI descriptor to the standard descriptor type.
func DescriptorFromOCI(desc OciDescriptor) (ocispec.Descriptor, error) {
	digestValue, err := ParseDigest(desc.Digest)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return ocispec.Descriptor{
		MediaType:   desc.MediaType,
		Digest:      digestValue,
		Size:        desc.Size,
		URLs:        desc.URLs,
		Annotations: desc.Annotations,
	}, nil
}

// ReadLocalBlob reads and verifies a content-addressed blob from an OCI layout.
func ReadLocalBlob(blobDir string, desc ocispec.Descriptor, maxSize int64) ([]byte, error) {
	if err := ValidateBlobSize(desc, maxSize); err != nil {
		return nil, err
	}
	digestValue, err := ParseDigest(desc.Digest.String())
	if err != nil {
		return nil, fmt.Errorf("parsing blob digest: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(blobDir, digestValue.Encoded()))
	if err != nil {
		return nil, fmt.Errorf("reading blob %s: %w", desc.Digest, err)
	}
	if err := VerifyBlob(data, desc); err != nil {
		return nil, err
	}
	return data, nil
}

// ValidateBlobSize rejects invalid or oversized metadata descriptors.
func ValidateBlobSize(desc ocispec.Descriptor, maxSize int64) error {
	if desc.Size < 0 {
		return fmt.Errorf("invalid metadata size %d for %s", desc.Size, desc.Digest)
	}
	if desc.Size > maxSize {
		return fmt.Errorf("metadata blob %s exceeds maximum allowed size of %d bytes", desc.Digest, maxSize)
	}
	return nil
}

// VerifyBlob checks a blob's declared size and digest.
func VerifyBlob(data []byte, desc ocispec.Descriptor) error {
	if int64(len(data)) != desc.Size {
		return fmt.Errorf("size mismatch for %s: expected %d, got %d", desc.Digest, desc.Size, len(data))
	}
	if got := digest.FromBytes(data); got != desc.Digest {
		return fmt.Errorf("digest mismatch for blob %s: got %s", desc.Digest, got)
	}
	return nil
}
