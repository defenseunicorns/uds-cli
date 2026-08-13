// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/errdef"
)

// mergeRootIndex builds the platform-keyed root index for the tag: the entry
// for child's architecture is replaced with child, other-arch bundle entries
// are preserved, and anything else at the tag is superseded. Entries are
// sorted by architecture for determinism.
func mergeRootIndex(ctx context.Context, dst oras.Target, tag string, child ocispec.Descriptor) ([]byte, ocispec.Descriptor, ocispec.Descriptor, error) {
	existing, currentRoot, err := existingRootEntries(ctx, dst, tag, child.Platform.Architecture)
	if err != nil {
		return nil, ocispec.Descriptor{}, ocispec.Descriptor{}, fmt.Errorf("reading existing root index at %s: %w", tag, err)
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
		return nil, ocispec.Descriptor{}, ocispec.Descriptor{}, fmt.Errorf("marshaling root index: %w", err)
	}
	return rootBytes, NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, rootBytes), currentRoot, nil
}

func existingRootEntries(ctx context.Context, dst oras.Target, tag, arch string) ([]ocispec.Descriptor, ocispec.Descriptor, error) {
	desc, err := dst.Resolve(ctx, tag)
	if err != nil {
		if errors.Is(err, errdef.ErrNotFound) {
			return nil, ocispec.Descriptor{}, nil
		}
		return nil, ocispec.Descriptor{}, fmt.Errorf("resolving %s: %w", tag, err)
	}
	data, err := fetchIndexBytes(ctx, dst, desc)
	if err != nil {
		return nil, ocispec.Descriptor{}, err
	}

	var idx ocispec.Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("parsing existing content at %s: %w", tag, err)
	}
	if idx.ArtifactType != "" {
		return nil, desc, nil
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
	return keep, desc, nil
}
