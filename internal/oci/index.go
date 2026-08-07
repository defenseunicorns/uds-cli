// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sort"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
)

// findBundleDefinitionEntry locates the bundle definition manifest entry in an OCI index.
// Returns the entry, its position in the Manifests slice, and any error.
func findBundleDefinitionEntry(idx ociIndex) (*ociManifest, int, error) {
	for i := range idx.Manifests {
		if idx.Manifests[i].ArtifactType == MediaTypeBundleDefinition {
			return &idx.Manifests[i], i, nil
		}
	}
	return nil, -1, fmt.Errorf("bundle definition manifest not found in index")
}

// isBundleIndex reports whether idx is a canonical single-arch UDS bundle
// (child) index, identified solely by its top-level artifactType (ADR-0015).
func isBundleIndex(idx ociIndex) bool {
	return idx.ArtifactType == MediaTypeBundle
}

// sortManifestsByDigest sorts index entries by digest in place, the
// deterministic ordering invariant for bundle child indexes (ADR-0015).
func sortManifestsByDigest(manifests []ociManifest) {
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Digest < manifests[j].Digest })
}

// newBundleIndex builds the canonical single-arch bundle (child) index per
// ADR-0015: self-identified by artifactType, arch recorded as an annotation,
// and manifests sorted by digest so the index digest is deterministic
// regardless of HCL package declaration order.
func newBundleIndex(manifests []ociManifest, arch string) *ociIndex {
	if arch == "" {
		arch = runtime.GOARCH
	}
	sorted := make([]ociManifest, len(manifests))
	copy(sorted, manifests)
	sortManifestsByDigest(sorted)
	return &ociIndex{
		SchemaVersion: 2,
		MediaType:     ocispec.MediaTypeImageIndex,
		ArtifactType:  MediaTypeBundle,
		Manifests:     sorted,
		Annotations: map[string]string{
			AnnotationBundleArchitecture: arch,
		},
	}
}

// maxIndexSize bounds index manifest reads; real indexes are well under 10s of KiB.
// See https://github.com/opencontainers/distribution-spec/blob/4fc4ecbefaaa6e4e1682f59f5ac445d076cf642d/spec.md?plain=1#L540
const maxIndexSize = 4 << 20 // 4 MiB

// mergeRootIndex builds the platform-keyed root index for the tag: the entry
// for child's architecture is replaced with child, other-arch bundle entries
// are preserved, and anything else at the tag is superseded. Entries are
// sorted by architecture for determinism.
func mergeRootIndex(ctx context.Context, dst oras.Target, tag string, child ocispec.Descriptor) ([]byte, ocispec.Descriptor, error) {
	existing, err := existingRootEntries(ctx, dst, tag, child.Platform.Architecture)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("reading existing root index at %s: %w", tag, err)
	}
	entries := append(existing, child)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Platform.Architecture < entries[j].Platform.Architecture
	})

	root := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: entries,
	}
	rootBytes, err := json.Marshal(&root)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("marshaling root index: %w", err)
	}
	return rootBytes, content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, rootBytes), nil
}

// existingRootEntries returns the other-arch bundle entries of the root index
// currently at the tag. A missing tag or a non-root artifact at the tag (e.g.
// a child bundle index) yields nil entries — the push then publishes a fresh
// root containing only the incoming architecture. Any other failure to read
// the existing root is an error: proceeding would silently clobber the other
// architectures' entries.
func existingRootEntries(ctx context.Context, dst oras.Target, tag, arch string) ([]ocispec.Descriptor, error) {
	desc, err := dst.Resolve(ctx, tag)
	if err != nil {
		if errors.Is(err, errdef.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolving %s: %w", tag, err)
	}
	data, err := fetchIndexBytes(ctx, dst, desc)
	if err != nil {
		return nil, err
	}

	var idx ocispec.Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parsing existing content at %s: %w", tag, err)
	}
	// A root index has no artifactType of its own; a child bundle index (or any
	// other typed artifact) at the tag is superseded rather than merged.
	if idx.ArtifactType != "" {
		return nil, nil
	}

	var keep []ocispec.Descriptor
	for _, m := range idx.Manifests {
		if m.MediaType != ocispec.MediaTypeImageIndex || m.ArtifactType != MediaTypeBundle {
			continue
		}
		if m.Platform == nil || m.Platform.Architecture == "" || m.Platform.Architecture == arch {
			continue
		}
		keep = append(keep, m)
	}
	return keep, nil
}

// resolveBundleChild resolves reference to the canonical single-arch bundle
// (child) index and returns its descriptor and raw bytes. A reference
// addressing a child directly (e.g. digest-pinned) is returned as-is; a tag
// pointing at a root index is platform-selected for arch (empty falls back to
// runtime.GOARCH). Anything else is an error.
func resolveBundleChild(ctx context.Context, src oras.Target, reference, arch string) (ocispec.Descriptor, []byte, error) {
	if arch == "" {
		arch = runtime.GOARCH
	}

	desc, err := src.Resolve(ctx, reference)
	if err != nil {
		return ocispec.Descriptor{}, nil, fmt.Errorf("resolving %s: %w", reference, err)
	}
	data, err := fetchIndexBytes(ctx, src, desc)
	if err != nil {
		return ocispec.Descriptor{}, nil, err
	}

	var idx ocispec.Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return ocispec.Descriptor{}, nil, fmt.Errorf("%s does not appear to be a UDS bundle: content is not an OCI index", reference)
	}

	// Direct child: the index self-identifies via artifactType.
	if idx.ArtifactType == MediaTypeBundle {
		return desc, data, nil
	}

	// Root index: select the child entry for the requested architecture.
	var available []string
	for _, m := range idx.Manifests {
		if m.MediaType != ocispec.MediaTypeImageIndex || m.ArtifactType != MediaTypeBundle || m.Platform == nil {
			continue
		}
		if m.Platform.Architecture != arch {
			available = append(available, m.Platform.Architecture)
			continue
		}
		childData, err := fetchIndexBytes(ctx, src, m)
		if err != nil {
			return ocispec.Descriptor{}, nil, err
		}
		var child ocispec.Index
		if err := json.Unmarshal(childData, &child); err != nil || child.ArtifactType != MediaTypeBundle {
			return ocispec.Descriptor{}, nil, fmt.Errorf("root index entry for %s does not reference a UDS bundle", arch)
		}
		return m, childData, nil
	}

	if len(available) > 0 {
		return ocispec.Descriptor{}, nil, fmt.Errorf("no bundle for architecture %q at %s; available: %v", arch, reference, available)
	}
	return ocispec.Descriptor{}, nil, fmt.Errorf("%s does not appear to be a UDS bundle: index does not declare artifactType %s", reference, MediaTypeBundle)
}

// fetchIndexBytes fetches and digest-verifies a manifest's raw bytes from src
// via oras' content.FetchAll, guarding against absurd descriptor sizes first.
func fetchIndexBytes(ctx context.Context, src oras.Target, desc ocispec.Descriptor) ([]byte, error) {
	if desc.Size > maxIndexSize {
		return nil, fmt.Errorf("index %s exceeds maximum allowed size of %d bytes", desc.Digest, maxIndexSize)
	}
	data, err := content.FetchAll(ctx, src, desc)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", desc.Digest, err)
	}
	return data, nil
}
