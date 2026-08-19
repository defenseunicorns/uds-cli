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

const (
	// Media types for UDS bundle OCI artifacts.
	MediaTypeBundleDefinition = "application/vnd.defenseunicorns.uds.bundle.definition.v1"
	MediaTypeBundleHCL        = "application/vnd.defenseunicorns.uds.bundle.hcl.v1"
	MediaTypeBundleValuesYAML = "application/vnd.defenseunicorns.uds.bundle.values.v1+yaml"

	// MediaTypeBundle is the artifactType of the canonical single-arch bundle
	// index (the child index a published tag's root index points at, and the
	// index.json inside a bundle .tar.zst). See ADR-0015.
	MediaTypeBundle = "application/vnd.defenseunicorns.uds.bundle.v1"

	// MediaTypeZarfLayer is the media type for Zarf package file layers.
	MediaTypeZarfLayer = "application/vnd.defenseunicorns.zarf.layer.v1"
	// AnnotationBundleArchitecture records the architecture of a child bundle index.
	AnnotationBundleArchitecture = "uds.dev/architecture"
	// AnnotationPackageName records the bundle package name that identifies a package descriptor.
	AnnotationPackageName = "uds.dev/package.name"
	// AnnotationPackageSource records the bundle package source for provenance.
	AnnotationPackageSource = "uds.dev/package.source"

	// AnnotationPackageVerification records a successful package verification during bundle creation.
	AnnotationPackageVerification = "uds.dev/package-verification"
	// AnnotationPackageVerificationVerified is the persisted value for a successful verification.
	AnnotationPackageVerificationVerified = "verified"
	// AnnotationReconfiguredFrom records the source bundle's child-index digest during reconfigure.
	AnnotationReconfiguredFrom = "org.defenseunicorns.uds.reconfigured-from"
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
