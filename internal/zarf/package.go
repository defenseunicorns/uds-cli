// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/cache"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// copySelectedPackage rewrites the package manifest before copying so excluded
// component content is never added to the bundle.
func copySelectedPackage(ctx context.Context, pkgLayout *layout.PackageLayout, selection []ocispec.Descriptor, dst *udsoci.Store, cacheDir string) (ocispec.Descriptor, error) {
	root, manifest, err := packageManifest(ctx, pkgLayout)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	manifest.Layers = selectedLayers(manifest.Layers, selection)
	return copyPackageManifest(ctx, pkgLayout, dst, root, manifest, cacheDir)
}

func packageManifest(ctx context.Context, pkgLayout *layout.PackageLayout) (ocispec.Descriptor, ocispec.Manifest, error) {
	packageName := pkgLayout.AsV1alpha1().Metadata.Name
	root, err := pkgLayout.Resolve(ctx, packageName)
	if err != nil {
		return ocispec.Descriptor{}, ocispec.Manifest{}, fmt.Errorf("%w for package %q: %w", ErrResolvePackageManifest, packageName, err)
	}
	manifest, err := pkgLayout.Manifest()
	if err != nil {
		return ocispec.Descriptor{}, ocispec.Manifest{}, fmt.Errorf("%w for package %q: %w", ErrReadPackageManifest, packageName, err)
	}
	return root, manifest.Manifest, nil
}

func copyPackageManifest(ctx context.Context, pkgLayout *layout.PackageLayout, dst *udsoci.Store, root ocispec.Descriptor, manifest ocispec.Manifest, cacheDir string) (ocispec.Descriptor, error) {
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w %s: %w", ErrMarshalPackageManifest, root.Digest, err)
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
			return ocispec.Descriptor{}, fmt.Errorf("copying package content %s: %w: %w", desc.Digest, ErrCopyPackageContent, err)
		}
	}
	if err := udsoci.PushDescriptorBytes(ctx, dst, root, manifestBytes); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("writing package manifest %s: %w: %w", root.Digest, ErrWritePackageManifest, err)
	}
	if cacheDir != "" {
		for _, layer := range manifest.Layers {
			if !isImageBlobLayer(layer) {
				continue
			}
			if err := cacheImageBlobLayer(ctx, dst, cacheDir, layer); err != nil {
				return ocispec.Descriptor{}, fmt.Errorf("caching package image layer %s: %w", layer.Digest, err)
			}
		}
	}
	return root, nil
}

func isImageBlobLayer(layer ocispec.Descriptor) bool {
	title := filepath.ToSlash(layer.Annotations[ocispec.AnnotationTitle])
	prefix := filepath.ToSlash(layout.ImagesBlobsDir) + "/"
	return strings.HasPrefix(title, prefix) && strings.TrimPrefix(title, prefix) == layer.Digest.Encoded()
}

func cacheImageBlobLayer(ctx context.Context, store *udsoci.Store, cacheDir string, layer ocispec.Descriptor) error {
	reader, err := store.Fetch(ctx, layer)
	if err != nil {
		return fmt.Errorf("reading ingested layer: %w", err)
	}
	writeErr := cache.WriteLayer(cacheDir, layer, reader)
	closeErr := reader.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return fmt.Errorf("closing ingested layer: %w", closeErr)
	}
	return nil
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
