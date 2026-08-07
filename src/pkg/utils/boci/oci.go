// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package boci (bundle OCI) provides OCI utility functions for bundles
package boci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
)

// ToOCIStore takes an arbitrary type, typically a struct, marshals it into JSON and store it in a local OCI store
func ToOCIStore(t any, mediaType string, store *ocistore.Store) (ocispec.Descriptor, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	desc := content.NewDescriptorFromBytes(mediaType, b)
	if exists, _ := store.Exists(context.Background(), desc); exists {
		return desc, nil
	}
	if err := store.Push(context.TODO(), desc, bytes.NewReader(b)); err != nil {
		return ocispec.Descriptor{}, err
	}
	return desc, nil
}

// ToOCIRemote takes an arbitrary type, typically a struct, marshals it into JSON and store it in a remote OCI store
func ToOCIRemote(t any, mediaType string, remote *oci.OrasRemote) (*ocispec.Descriptor, error) {
	ctx := context.TODO()
	b, err := json.Marshal(t)
	if err != nil {
		return &ocispec.Descriptor{}, err
	}

	var layerDesc *ocispec.Descriptor
	// if image manifest media type, push to Manifests(), otherwise normal pushLayer()
	if mediaType == ocispec.MediaTypeImageManifest {
		descriptorFromBytes := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, b)
		layerDesc = &descriptorFromBytes
		if err := remote.Repo().Manifests().PushReference(ctx, descriptorFromBytes, bytes.NewReader(b), remote.Repo().Reference.String()); err != nil {
			return &ocispec.Descriptor{}, fmt.Errorf("failed to push manifest: %w", err)
		}
	} else {
		layerDesc, err = remote.PushLayer(ctx, b, mediaType)
		if err != nil {
			return &ocispec.Descriptor{}, err
		}
	}

	message.Successf("Published %s [%s]", remote.Repo().Reference.String(), layerDesc.MediaType)
	return layerDesc, nil
}

// CreateCopyOpts creates the ORAS CopyOpts struct to use when copying OCI artifacts
func CreateCopyOpts(layersToPull []ocispec.Descriptor, concurrency int) oras.CopyOptions {
	var copyOpts oras.CopyOptions
	copyOpts.Concurrency = concurrency
	requestedDigests := map[string]struct{}{}
	for _, layer := range layersToPull {
		if layer.Digest != "" {
			requestedDigests[layer.Digest.String()] = struct{}{}
		}
	}
	copyOpts.FindSuccessors = func(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		var nodes []ocispec.Descriptor
		_, hasTitleAnnotation := desc.Annotations[ocispec.AnnotationTitle]
		_, isRequested := requestedDigests[desc.Digest.String()]

		if desc.MediaType == ocispec.MediaTypeImageIndex && !isRequested {
			// This block is triggered when ORAS initially hits the OCI repo and gets the image index (index.json)
			// and it grabs the bundle root manifest corresponding to the proper arch. Embedded image indexes
			// are included in requestedDigests and must retain all requested platform manifests.

			// get contents of the index.json from its desc
			successors, err := content.Successors(ctx, fetcher, desc)
			if err != nil {
				return nil, err
			}

			// grab the proper bundle root manifest, based on arch
			for _, node := range successors {
				if node.Platform.Architecture == config.GetArch() {
					return []ocispec.Descriptor{node}, nil
				}
			}
		} else if desc.MediaType == layout.ZarfLayerMediaTypeBlob && !hasTitleAnnotation {
			// This if block is for used for finding successors from bundle root manifests during bundle pull/publish ops;
			// note that ptrs to the Zarf pkg image manifests won't have title annotations, and will follow this code path
			// adopted from the content.Successors() fn in oras
			layerBytes, err := content.FetchAll(ctx, fetcher, desc)
			if err != nil {
				return nil, err
			}
			var manifest oci.Manifest
			if err := json.Unmarshal(layerBytes, &manifest); err != nil {
				return nil, err
			}
			if manifest.Subject != nil {
				nodes = append(nodes, *manifest.Subject)
			}
			nodes = append(nodes, manifest.Config)
			nodes = append(nodes, manifest.Layers...)
		} else {
			// this block is meant for pulling Zarf OCI pkgs directly, it also gets called as ORAS navigates the bundle root manifest
			successors, err := content.Successors(ctx, fetcher, desc)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, successors...)
		}
		var ret []ocispec.Descriptor
		for _, node := range nodes {
			_, isRequested := requestedDigests[node.Digest.String()]
			if node.Size != 0 && isRequested {
				ret = append(ret, node)
			}
		}
		return ret, nil
	}
	return copyOpts
}

// createIndex creates an OCI index and pushes it to a remote based on ref
func createIndex(bundle *types.UDSBundle, rootManifestDesc ocispec.Descriptor) *ocispec.Index {
	var index ocispec.Index
	index.MediaType = ocispec.MediaTypeImageIndex
	index.SchemaVersion = 2
	index.Manifests = []ocispec.Descriptor{
		{
			MediaType: ocispec.MediaTypeImageManifest,
			Digest:    rootManifestDesc.Digest,
			Size:      rootManifestDesc.Size,
			Platform: &ocispec.Platform{
				Architecture: bundle.Metadata.Architecture,
				OS:           oci.MultiOS,
			},
		},
	}
	return &index
}

// addToIndex adds or replaces a bundle root manifest to an OCI index
func addToIndex(index *ocispec.Index, bundle *types.UDSBundle, newManifestDesc ocispec.Descriptor) *ocispec.Index {
	manifestExists := false
	for i, manifest := range index.Manifests {
		// if existing manifest has the same arch as the bundle, don't append new bundle root manifest to index
		if manifest.Platform != nil && manifest.Platform.Architecture == bundle.Metadata.Architecture {
			// update digest and size in case they changed with the new bundle root manifest
			index.Manifests[i].Digest = newManifestDesc.Digest
			index.Manifests[i].Size = newManifestDesc.Size
			manifestExists = true
		}
	}
	if !manifestExists {
		newManifestDesc.Platform = &ocispec.Platform{
			Architecture: bundle.Metadata.Architecture,
			OS:           oci.MultiOS,
		}
		index.Manifests = append(index.Manifests, newManifestDesc)
	}
	return index
}

func pushIndex(index *ocispec.Index, remote *oci.OrasRemote, ref string) error {
	indexBytes, err := json.Marshal(index)
	if err != nil {
		return err
	}
	indexDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, indexBytes)
	err = remote.Repo().Manifests().PushReference(context.TODO(), indexDesc, bytes.NewReader(indexBytes), ref)
	if err != nil {
		return err
	}
	return nil
}

// UpdateIndex updates or creates a new OCI index based on the index arg, then pushes to the remote OCI repo
func UpdateIndex(index *ocispec.Index, remote *oci.OrasRemote, bundle *types.UDSBundle, newManifestDesc ocispec.Descriptor) error {
	var newIndex *ocispec.Index
	ref := bundle.Metadata.Version
	if index == nil {
		newIndex = createIndex(bundle, newManifestDesc)
	} else {
		newIndex = addToIndex(index, bundle, newManifestDesc)
	}
	err := pushIndex(newIndex, remote, ref)
	if err != nil {
		return err
	}
	return nil
}

// GetIndex gets the OCI index from a remote repository if the index exists, otherwise returns a
func GetIndex(remote *oci.OrasRemote, ref string) (*ocispec.Index, error) {
	ctx := context.TODO()
	var index *ocispec.Index
	existingRootDesc, err := remote.Repo().Resolve(ctx, ref)
	if err != nil {
		// ErrNotFound indicates that the repo hasn't been created yet, expected for brand new repos in a registry
		// if the err isn't of type ErrNotFound, it's a real error so return it
		if !errors.Is(err, errdef.ErrNotFound) {
			return nil, err
		}
	}
	// if an index exists, save it so we can update it after pushing the bundle's root manifest
	if existingRootDesc.MediaType == ocispec.MediaTypeImageIndex {
		rc, err := remote.Repo().Fetch(ctx, existingRootDesc)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		b, err := content.ReadAll(rc, existingRootDesc)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &index); err != nil {
			return nil, err
		}
	}
	return index, nil
}

// EnsureOCIPrefix ensures oci prefix is part of provided remote source path, and adds it if it's not
func EnsureOCIPrefix(source string) string {
	var ociPrefix = "oci://"
	if !strings.Contains(source, ociPrefix) {
		return ociPrefix + source
	}
	return source
}

// SelectPackageContent returns the Zarf package content needed for the required and selected optional components.
// Path-bearing descriptors are preserved even when multiple paths reference the same content digest.
func SelectPackageContent(ctx context.Context, manifest *oci.Manifest, fetcher content.Fetcher, optionalComponents []string, include ...zoci.LayerType) ([]ocispec.Descriptor, error) {
	pkg, err := zoci.FetchZarfYAML(ctx, manifest, fetcher)
	if err != nil {
		return nil, fmt.Errorf("fetching zarf package metadata: %w", err)
	}

	components, err := filters.ForDeploy(strings.Join(optionalComponents, ","), false).Apply(pkg)
	if err != nil {
		return nil, fmt.Errorf("selecting zarf package components: %w", err)
	}
	if len(include) == 0 {
		include = zoci.GetAllLayerTypes()
	}
	if pkg.Build.Architecture == v1alpha1.SkeletonArch {
		include = helpers.RemoveMatches(include, func(layerType zoci.LayerType) bool {
			return layerType == zoci.ImageLayers
		})
	}

	layers, err := zoci.AssembleLayers(ctx, manifest, fetcher, components, include...)
	if err != nil {
		return nil, fmt.Errorf("assembling zarf package layers: %w", err)
	}
	layers = append(layers, manifest.Config)
	selected := make([]ocispec.Descriptor, 0, len(layers))
	for _, desc := range layers {
		if oci.IsEmptyDescriptor(desc) {
			continue
		}
		if err := desc.Digest.Validate(); err != nil {
			return nil, fmt.Errorf("selected descriptor has invalid digest %q: %w", desc.Digest, err)
		}
		selected = append(selected, desc)
	}
	return selected, nil
}

// SelectBundledPackageContent returns selected package content plus its manifest descriptor from a bundle manifest.
func SelectBundledPackageContent(ctx context.Context, bundleManifest *oci.Manifest, fetcher content.Fetcher, manifestDigest digest.Digest, optionalComponents []string) ([]ocispec.Descriptor, error) {
	manifestDesc := bundleManifest.Locate(manifestDigest.Encoded())
	if oci.IsEmptyDescriptor(manifestDesc) {
		return nil, fmt.Errorf("package manifest digest %s does not exist in bundle", manifestDigest)
	}
	manifest, err := oci.FetchUnmarshal[oci.Manifest](ctx, fetcher, json.Unmarshal, manifestDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching package manifest %s: %w", manifestDigest, err)
	}
	selected, err := SelectPackageContent(ctx, &manifest, fetcher, optionalComponents)
	if err != nil {
		return nil, err
	}
	return append(selected, manifestDesc), nil
}

// MaterializePackagePaths writes any selected titled descriptor that a content-deduplicating copy skipped.
func MaterializePackagePaths(ctx context.Context, fetcher content.Fetcher, destination string, descriptors []ocispec.Descriptor) error {
	for _, desc := range descriptors {
		title := desc.Annotations[ocispec.AnnotationTitle]
		if title == "" {
			continue
		}
		cleanPath := filepath.Clean(filepath.FromSlash(title))
		if filepath.IsAbs(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid package path %q", title)
		}
		target := filepath.Join(destination, cleanPath)
		if _, err := os.Stat(target); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		reader, err := fetcher.Fetch(ctx, desc)
		if err != nil {
			return fmt.Errorf("fetching package path %q: %w", title, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			reader.Close()
			return err
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			reader.Close()
			return err
		}
		_, copyErr := io.Copy(file, reader)
		closeErr := errors.Join(file.Close(), reader.Close())
		if err := errors.Join(copyErr, closeErr); err != nil {
			_ = os.Remove(target)
			return fmt.Errorf("writing package path %q: %w", title, err)
		}
	}
	return nil
}

// CopyLayers uses ORAS to copy layers from a remote repo to a local OCI store
func CopyLayers(layersToPull []ocispec.Descriptor, estimatedBytes int64, tmpDstDir string, repo *remote.Repository, target oras.Target, artifactName string) (ocispec.Descriptor, error) { //nolint:revive
	// copy Zarf pkg
	copyOpts := CreateCopyOpts(layersToPull, config.CommonOptions.OCIConcurrency)
	// Create a thread to update a progress bar as we save the package to disk
	doneSaving := make(chan error)

	// Grab tmpDirSize and add it to the estimatedBytes, otherwise the progress bar will be off
	// because as multiple packages are pulled into the tmpDir, RenderProgressBarForLocalDirWrite continues to
	// add their size which results in strange MB ratios
	tmpDirSize, err := helpers.GetDirSize(tmpDstDir)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	expectedTotalSize := estimatedBytes + tmpDirSize

	go utils.RenderProgressBarForLocalDirWrite(tmpDstDir, expectedTotalSize, doneSaving, "Pulling: "+artifactName, "Successfully pulled: "+artifactName)

	rootDesc, err := oras.Copy(context.TODO(), repo, repo.Reference.String(), target, "", copyOpts)

	doneSaving <- err
	<-doneSaving

	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return rootDesc, nil
}
