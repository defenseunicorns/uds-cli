// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	return ocispec.Descriptor{}, -1, fmt.Errorf("bundle definition manifest not found in index")
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
	return os.WriteFile(path, append(b, '\n'), tmpFilePerm)
}

// packageRootDescriptor returns the sole root descriptor from a Zarf package layout.
func packageRootDescriptor(ociDir string) (ocispec.Descriptor, error) {
	data, err := os.ReadFile(filepath.Join(ociDir, ocispec.ImageIndexFile))
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("reading index.json: %w", err)
	}
	var idx ocispec.Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("parsing index.json: %w", err)
	}
	if len(idx.Manifests) != 1 {
		return ocispec.Descriptor{}, fmt.Errorf("index.json contains %d manifests; expected exactly 1 for a Zarf package layout", len(idx.Manifests))
	}
	return idx.Manifests[0], nil
}

// TaggedDerivativeReference returns source tag, target tag, and target reference for a suffixed derivative tag.
func TaggedDerivativeReference(source, suffix string) (string, string, string, error) {
	ref, err := registry.ParseReference(TrimScheme(source))
	if err != nil {
		return "", "", "", fmt.Errorf("parsing OCI reference: %w", err)
	}
	if ref.Reference == "" || strings.Contains(ref.Reference, ":") {
		return "", "", "", fmt.Errorf("OCI source must use a tag reference (e.g. :v1.0.0), not a digest")
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
