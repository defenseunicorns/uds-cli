// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
)

// ResolveBundleChild resolves reference to the canonical single-arch bundle
// index and returns its descriptor and raw bytes.
func ResolveBundleChild(ctx context.Context, src oras.Target, reference, arch string) (ocispec.Descriptor, []byte, error) {
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
	if idx.ArtifactType == MediaTypeBundle {
		return desc, data, nil
	}

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

func fetchIndexBytes(ctx context.Context, src oras.Target, desc ocispec.Descriptor) ([]byte, error) {
	return FetchBytes(ctx, src, desc)
}
