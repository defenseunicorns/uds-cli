// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package boci

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
)

// PackageManifestLayerDescriptor returns the opaque descriptor used for a package manifest in a bundle root.
// Only the parent-facing media type changes; the target remains the original OCI manifest bytes and digest.
// It is intentionally bare because bundle graph traversal identifies package manifests by the absence of a title.
func PackageManifestLayerDescriptor(sourceDesc ocispec.Descriptor) ocispec.Descriptor {
	return ocispec.Descriptor{
		MediaType: layout.ZarfLayerMediaTypeBlob,
		Digest:    sourceDesc.Digest,
		Size:      sourceDesc.Size,
	}
}

// PushPackageManifest verifies and pushes the original package manifest bytes as an opaque bundle layer.
func PushPackageManifest(ctx context.Context, target content.Pusher, sourceDesc ocispec.Descriptor, manifestBytes []byte) (ocispec.Descriptor, error) {
	if err := sourceDesc.Digest.Validate(); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("invalid source package manifest digest: %w", err)
	}
	verifier := sourceDesc.Digest.Verifier()
	if _, err := verifier.Write(manifestBytes); err != nil {
		return ocispec.Descriptor{}, err
	}
	if !verifier.Verified() || int64(len(manifestBytes)) != sourceDesc.Size {
		return ocispec.Descriptor{}, fmt.Errorf("package manifest content does not match source descriptor %s", sourceDesc.Digest)
	}
	blobDesc := PackageManifestLayerDescriptor(sourceDesc)
	if err := target.Push(ctx, blobDesc, bytes.NewReader(manifestBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, err
	}
	return blobDesc, nil
}
