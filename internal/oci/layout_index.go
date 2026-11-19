// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/filesystem"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry"
)

// IsBundleIndex reports whether idx is a canonical bundle index.
func IsBundleIndex(idx ocispec.Index) bool {
	return idx.ArtifactType == MediaTypeBundle
}

// FindBundleDefinition locates the bundle definition manifest in a spec index.
func FindBundleDefinition(idx ocispec.Index) (ocispec.Descriptor, int, error) {
	for i, manifest := range idx.Manifests {
		if manifest.ArtifactType == MediaTypeBundleDefinition {
			return manifest, i, nil
		}
	}
	return ocispec.Descriptor{}, -1, ErrBundleDefinitionNotFound
}

// SortDescriptors sorts descriptors deterministically by digest and metadata.
func SortDescriptors(manifests []ocispec.Descriptor) {
	sort.Slice(manifests, func(i, j int) bool {
		return descriptorSortKey(manifests[i]) < descriptorSortKey(manifests[j])
	})
}

func descriptorSortKey(desc ocispec.Descriptor) string {
	annotations, _ := json.Marshal(desc.Annotations)
	platform, _ := json.Marshal(desc.Platform)
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%s\x00%s",
		desc.Digest,
		desc.Annotations[ocispec.AnnotationRefName],
		desc.MediaType,
		desc.ArtifactType,
		desc.Size,
		annotations,
		platform,
	)
}

// WriteIndex writes an OCI image index.
func WriteIndex(path string, idx *ocispec.Index) error {
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), filesystem.PrivateFileMode)
}

// TaggedDerivativeReference returns source tag, target tag, and target reference for a suffixed derivative tag.
func TaggedDerivativeReference(source, suffix string) (string, string, string, error) {
	ref, err := registry.ParseReference(TrimScheme(source))
	if err != nil {
		return "", "", "", fmt.Errorf("%w %q: %w", ErrParseReference, source, err)
	}
	if ref.Reference == "" || strings.Contains(ref.Reference, ":") {
		return "", "", "", ErrTagReferenceRequired
	}
	sourceTag := ref.Reference
	targetTag := sourceTag + suffix
	return sourceTag, targetTag, "oci://" + ref.Registry + "/" + ref.Repository + ":" + targetTag, nil
}

// NewBundleIndex builds a deterministic single-architecture bundle index.
func NewBundleIndex(manifests []ocispec.Descriptor, arch string) *ocispec.Index {
	SortDescriptors(manifests)
	return &ocispec.Index{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageIndex,
		ArtifactType: MediaTypeBundle,
		Manifests:    manifests,
		Annotations: map[string]string{
			AnnotationBundleArchitecture: arch,
		},
	}
}
