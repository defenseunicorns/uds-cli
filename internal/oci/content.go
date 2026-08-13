// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	packageoci "github.com/defenseunicorns/pkg/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
)

const (
	mediaTypeDockerManifest = "application/vnd.docker.distribution.manifest.v2+json"
	// MaxFetchBytesSize is the largest descriptor UDS CLI will buffer in memory.
	MaxFetchBytesSize = 16 << 20
)

// IsImageManifestMediaType reports whether mediaType identifies an OCI or Docker image manifest.
func IsImageManifestMediaType(mediaType string) bool {
	return mediaType == ocispec.MediaTypeImageManifest || mediaType == mediaTypeDockerManifest
}

// FetchBytes fetches descriptor content, verifies it against the descriptor
// digest and size, and returns the full content in memory.
//
// This is the buffered path. Use it only when the caller needs the complete
// []byte value, such as for OCI indexes, manifests, configs, or bundle
// definition layers. The size limit is checked before reading so a registry or
// archive cannot force unbounded memory use.
//
// Do not use FetchBytes for package layers or arbitrary content blobs. Those can
// be large and should be copied by ORAS graph operations or streamed through
// Fetch with content.NewVerifyReader when verification is needed.
//
// The fetcher passed to FetchBytes should not perform eager body reads before
// FetchBytes is called. For metadata-only reads from a local OCI layout, open the
// layout with OpenReadOnlyStore instead of OpenStore.
func FetchBytes(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) ([]byte, error) {
	if desc.Size > MaxFetchBytesSize {
		return nil, fmt.Errorf("descriptor %s is %d bytes, larger than the %d byte buffered fetch limit", desc.Digest, desc.Size, MaxFetchBytesSize)
	}
	data, err := content.FetchAll(ctx, fetcher, desc)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", desc.Digest, err)
	}
	return data, nil
}

// NewDescriptorFromBytes returns the descriptor for data using ORAS digest and
// size calculation.
func NewDescriptorFromBytes(mediaType string, data []byte) ocispec.Descriptor {
	return content.NewDescriptorFromBytes(mediaType, data)
}

// PushDescriptorBytes stores data for desc and treats an existing content-addressed
// blob as success.
func PushDescriptorBytes(ctx context.Context, pusher content.Pusher, desc ocispec.Descriptor, data []byte) error {
	if err := pusher.Push(ctx, desc, bytes.NewReader(data)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return err
	}
	return nil
}

// PushBytes stores data and returns the descriptor that addresses it.
func PushBytes(ctx context.Context, pusher content.Pusher, mediaType string, data []byte, annotations map[string]string) (ocispec.Descriptor, error) {
	desc := NewDescriptorFromBytes(mediaType, data)
	desc.Annotations = annotations
	if err := PushDescriptorBytes(ctx, pusher, desc, data); err != nil {
		return ocispec.Descriptor{}, err
	}
	return desc, nil
}

// PushManifestBytes stores manifest bytes and returns a descriptor annotated with
// the manifest artifact type.
func PushManifestBytes(ctx context.Context, pusher content.Pusher, mediaType, artifactType string, data []byte) (ocispec.Descriptor, error) {
	desc := NewDescriptorFromBytes(mediaType, data)
	desc.ArtifactType = artifactType
	if err := PushDescriptorBytes(ctx, pusher, desc, data); err != nil {
		return ocispec.Descriptor{}, err
	}
	return desc, nil
}

// CopyGraph copies the graph rooted at root from src to dst using ORAS defaults.
func CopyGraph(ctx context.Context, src content.ReadOnlyStorage, dst content.Storage, root ocispec.Descriptor) error {
	return copyGraph(ctx, src, dst, root, oras.DefaultCopyGraphOptions)
}

func copyGraph(ctx context.Context, src content.ReadOnlyStorage, dst content.Storage, root ocispec.Descriptor, opts oras.CopyGraphOptions) error {
	if err := oras.CopyGraph(ctx, src, dst, root, opts); err != nil {
		return fmt.Errorf("copying graph %s: %w", root.Digest, err)
	}
	return nil
}

func copyReference(ctx context.Context, src oras.ReadOnlyTarget, srcRef string, dst oras.Target, dstRef string, opts oras.CopyOptions) (ocispec.Descriptor, error) {
	desc, err := oras.Copy(ctx, src, srcRef, dst, dstRef, opts)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return desc, nil
}

// PushReferenceBytes stores manifest bytes at reference. Remote ORAS
// repositories need PushReference so the bytes go to the manifest endpoint;
// generic Push writes to the blob store there. Local/test targets that do not
// expose PushReference fall back to Push plus Tag.
func PushReferenceBytes(ctx context.Context, target Target, desc ocispec.Descriptor, data []byte, reference string) error {
	if pusher, ok := target.(interface {
		PushReference(context.Context, ocispec.Descriptor, io.Reader, string) error
	}); ok {
		return pusher.PushReference(ctx, desc, bytes.NewReader(data), reference)
	}
	if err := PushDescriptorBytes(ctx, target, desc, data); err != nil {
		return err
	}
	return Tag(ctx, target, desc, reference)
}

// Tag records reference for desc on target.
func Tag(ctx context.Context, target interface {
	Tag(context.Context, ocispec.Descriptor, string) error
}, desc ocispec.Descriptor, reference string) error {
	return target.Tag(ctx, desc, reference)
}

// EnsureTagAvailable returns an error when target already has tag or cannot check it.
func EnsureTagAvailable(ctx context.Context, target Target, tag string) error {
	if _, err := target.Resolve(ctx, tag); err == nil {
		return fmt.Errorf("target tag %q already exists in registry", tag)
	} else if !IsNotFound(err) {
		return fmt.Errorf("checking target tag %q: %w", tag, err)
	}
	return nil
}

// BundleChildDescriptor returns desc annotated as a bundle child index for arch.
func BundleChildDescriptor(desc ocispec.Descriptor, arch string) ocispec.Descriptor {
	desc.ArtifactType = MediaTypeBundle
	desc.Platform = &ocispec.Platform{Architecture: arch, OS: packageoci.MultiOS}
	return desc
}

// PublishBundleRootIndex wraps child in the multi-architecture root index at tag.
func PublishBundleRootIndex(ctx context.Context, target Target, tag string, child ocispec.Descriptor) error {
	rootBytes, rootDesc, currentRoot, err := mergeRootIndex(ctx, target, tag, child)
	if err != nil {
		return err
	}
	if currentRoot.Digest == rootDesc.Digest {
		return nil
	}
	if err := PushReferenceBytes(ctx, target, rootDesc, rootBytes, tag); err != nil {
		return fmt.Errorf("pushing root index as %s: %w", tag, err)
	}
	return nil
}

// PackBundleDefinitionManifest stores the bundle definition artifact manifest.
func PackBundleDefinitionManifest(ctx context.Context, store content.Storage, layers []ocispec.Descriptor) (ocispec.Descriptor, error) {
	desc, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, MediaTypeBundleDefinition, oras.PackManifestOptions{
		Layers: layers,
		ManifestAnnotations: map[string]string{
			ocispec.AnnotationCreated: "1970-01-01T00:00:00Z",
		},
	})
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	desc.ArtifactType = MediaTypeBundleDefinition
	return desc, nil
}
