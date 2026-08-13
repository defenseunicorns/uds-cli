// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"encoding/json"
	"fmt"

	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// copySelectedPackage rewrites the package manifest before copying so excluded
// component content is never added to the bundle.
func copySelectedPackage(ctx context.Context, pkgLayout *layout.PackageLayout, selection []ocispec.Descriptor, dst *udsoci.Store) (ocispec.Descriptor, error) {
	root, manifest, err := packageManifest(ctx, pkgLayout)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	manifest.Layers = selectedLayers(manifest.Layers, selection)
	return copyPackageManifest(ctx, pkgLayout, dst, root, manifest)
}

func packageManifest(ctx context.Context, pkgLayout *layout.PackageLayout) (ocispec.Descriptor, ocispec.Manifest, error) {
	root, err := pkgLayout.Resolve(ctx, pkgLayout.Pkg.Metadata.Name)
	if err != nil {
		return ocispec.Descriptor{}, ocispec.Manifest{}, fmt.Errorf("resolving package manifest: %w", err)
	}
	manifest, err := pkgLayout.Manifest()
	if err != nil {
		return ocispec.Descriptor{}, ocispec.Manifest{}, fmt.Errorf("reading package manifest: %w", err)
	}
	return root, manifest.Manifest, nil
}

func copyPackageManifest(ctx context.Context, pkgLayout *layout.PackageLayout, dst *udsoci.Store, root ocispec.Descriptor, manifest ocispec.Manifest) (ocispec.Descriptor, error) {
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshaling package manifest: %w", err)
	}
	root = udsoci.NewDescriptorFromBytes(root.MediaType, manifestBytes)
	root.ArtifactType = manifest.ArtifactType
	root.Annotations = manifest.Annotations

	contentDescriptors := append([]ocispec.Descriptor{manifest.Config}, manifest.Layers...)
	for _, desc := range contentDescriptors {
		if desc.Digest == "" {
			continue
		}
		if err := udsoci.CopyGraph(ctx, pkgLayout, dst, desc); err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("copying package content %s: %w", desc.Digest, err)
		}
	}
	if err := udsoci.PushDescriptorBytes(ctx, dst, root, manifestBytes); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("writing package manifest %s: %w", root.Digest, err)
	}
	return root, nil
}

func selectedLayers(layers, selection []ocispec.Descriptor) []ocispec.Descriptor {
	selected := make(map[layerIdentity]struct{}, len(selection))
	for _, desc := range selection {
		selected[newLayerIdentity(desc)] = struct{}{}
	}
	result := make([]ocispec.Descriptor, 0, len(selection))
	for _, layer := range layers {
		if _, ok := selected[newLayerIdentity(layer)]; ok {
			result = append(result, layer)
		}
	}
	return result
}

func newLayerIdentity(desc ocispec.Descriptor) layerIdentity {
	return layerIdentity{digest: desc.Digest.String(), title: desc.Annotations[ocispec.AnnotationTitle]}
}
