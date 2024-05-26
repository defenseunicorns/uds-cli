// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package boci (bundle OCI) provides OCI utility functions for bundles
package boci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/packager/filters"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
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
	var shas []string
	for _, layer := range layersToPull {
		if len(layer.Digest.String()) > 0 {
			shas = append(shas, layer.Digest.Encoded())
		}
	}
	copyOpts.FindSuccessors = func(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		var nodes []ocispec.Descriptor
		_, hasTitleAnnotation := desc.Annotations[ocispec.AnnotationTitle]

		if desc.MediaType == ocispec.MediaTypeImageIndex {
			// This block is triggered when ORAS initially hits the OCI repo and gets the image index (index.json)
			// and it grabs the bundle root manifest corresponding to the proper arch
			// todo: refactor to solve the arch problem using the shas var above instead of checking here

			// get contents of the index.json from its desc
			successors, err := content.Successors(ctx, fetcher, desc)
			if err != nil {
				return nil, err
			}

			// grab the proper bundle root manifest, based on arch
			for _, node := range successors {
				// todo: remove this check once we have a better way to handle arch
				if node.Platform.Architecture == config.GetArch() {
					return []ocispec.Descriptor{node}, nil
				}
			}
		} else if desc.MediaType == zoci.ZarfLayerMediaTypeBlob && !hasTitleAnnotation {
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
			if node.Size != 0 && slices.Contains(shas, node.Digest.Encoded()) {
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
	index.Versioned.SchemaVersion = 2
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

// FindPkgLayers finds the necessary Zarf pkg layers from a remote OCI registry
func FindPkgLayers(remote zoci.Remote, pkgRootManifest *oci.Manifest, optionalComponents []string) ([]ocispec.Descriptor, error) {
	ctx := context.TODO()
	zarfPkg, err := remote.FetchZarfYAML(ctx)
	if err != nil {
		return nil, err
	}

	// ensure we're only pulling required components and optional components and images
	var components []zarfTypes.ZarfComponent
	for _, c := range zarfPkg.Components {
		if c.Required != nil || slices.Contains(optionalComponents, c.Name) {
			components = append(components, c)
		}
	}
	layersFromComponents, err := remote.LayersFromRequestedComponents(ctx, components)
	if err != nil {
		return nil, err
	}

	// get the layers that are always pulled
	var metadataLayers []ocispec.Descriptor
	for _, path := range zoci.PackageAlwaysPull {
		layer := pkgRootManifest.Locate(path)
		if !oci.IsEmptyDescriptor(layer) {
			metadataLayers = append(metadataLayers, layer)
		}
	}
	layersToCopy := append(layersFromComponents, metadataLayers...)
	layersToCopy = append(layersToCopy, pkgRootManifest.Config)
	return layersToCopy, err
}

// FilterImageIndex filters out optional components from the images index
func FilterImageIndex(components []zarfTypes.ZarfComponent, imgIndex ocispec.Index) ([]ocispec.Descriptor, error) {
	// include only images that are in the components using a map to dedup manifests
	manifestIncludeMap := map[string]ocispec.Descriptor{}
	for _, manifest := range imgIndex.Manifests {
		for _, component := range components {
			for _, imgName := range component.Images {
				// include backwards compatibility shim for older Zarf versions that would leave docker.io off of image annotations
				if manifest.Annotations[ocispec.AnnotationBaseImageName] == imgName ||
					manifest.Annotations[ocispec.AnnotationBaseImageName] == fmt.Sprintf("docker.io/%s", imgName) {
					manifestIncludeMap[manifest.Digest.Hex()] = manifest
				}
			}
		}
	}
	// convert map to list and rewrite the index manifests
	var manifestsToInclude []ocispec.Descriptor
	for _, manifest := range manifestIncludeMap {
		manifestsToInclude = append(manifestsToInclude, manifest)
	}

	return manifestsToInclude, nil
}

// FindBundledPkgLayers finds the necessary Zarf pkg layers from a remote bundle
func FindBundledPkgLayers(ctx context.Context, pkg types.Package, rootManifest *oci.Manifest, remote *oci.OrasRemote) ([]ocispec.Descriptor, int64, error) {
	var layersToPull []ocispec.Descriptor
	estPkgBytes := int64(0)

	// grab sha of zarf image manifest and pull it down
	sha := strings.Split(pkg.Ref, "@sha256:")[1] // this is where we use the SHA appended to the Zarf pkg inside the bundle
	manifestDesc := rootManifest.Locate(sha)
	manifestBytes, err := remote.FetchLayer(ctx, manifestDesc)
	if err != nil {
		return nil, 0, err
	}

	// unmarshal the zarf image manifest and add it to the layers to pull
	var manifest oci.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, 0, err
	}

	layersToPull = append(layersToPull, manifestDesc)

	filteredComponents, err := getFilteredComponents(ctx, remote, manifest, pkg.OptionalComponents)
	if err != nil {
		return nil, 0, err
	}

	// go through manifest layers and add to layersToPull as appropriate
	var imgIndex ocispec.Index
	for _, desc := range manifest.Layers {
		descAnnotationTile := desc.Annotations[ocispec.AnnotationTitle]
		if descAnnotationTile == "images/index.json" {
			imgIndex, err = handleImgIndex(ctx, remote, desc)
			if err != nil {
				return nil, 0, err
			}
			layersToPull = append(layersToPull, desc)
		} else if strings.HasPrefix(descAnnotationTile, "components/") {
			if shouldInclude := utils.IncludeComponent(descAnnotationTile, filteredComponents); shouldInclude {
				layersToPull = append(layersToPull, desc)
			}
		} else if !strings.Contains(descAnnotationTile, config.BlobsDir) {
			// not a blob or component, add to layersToPull
			layersToPull = append(layersToPull, desc)
		}
	}

	// get only image manifests that are part of req'd + selected components
	manifestsToInclude, err := FilterImageIndex(filteredComponents, imgIndex)
	if err != nil {
		return nil, 0, err
	}

	// grab all layers from the included image manifests
	for _, desc := range manifestsToInclude {
		imgManifest, err := getImgManifest(ctx, remote, desc)
		if err != nil {
			return nil, 0, err
		}

		// grab all layers in image manifest, the img config and the img manifest itself
		layersToPull = append(layersToPull, imgManifest.Layers...)
		layersToPull = append(layersToPull, desc, imgManifest.Config)
	}

	// loop through layersToPull and add up bytes
	for _, layer := range layersToPull {
		estPkgBytes += layer.Size
	}

	return layersToPull, estPkgBytes, nil
}

func getImgManifest(ctx context.Context, remote *oci.OrasRemote, desc ocispec.Descriptor) (ocispec.Manifest, error) {
	imgManifestReader, err := remote.Repo().Blobs().Fetch(ctx, desc)
	if err != nil {
		return ocispec.Manifest{}, err
	}
	imgManifestBytes, err := io.ReadAll(imgManifestReader)
	if err != nil {
		return ocispec.Manifest{}, err
	}
	var imgManifest ocispec.Manifest
	if err := json.Unmarshal(imgManifestBytes, &imgManifest); err != nil {
		return ocispec.Manifest{}, err
	}
	err = imgManifestReader.Close()
	if err != nil {
		return ocispec.Manifest{}, err
	}
	return imgManifest, nil
}

func handleImgIndex(ctx context.Context, remote *oci.OrasRemote, desc ocispec.Descriptor) (ocispec.Index, error) {
	indexBytes, err := remote.FetchLayer(ctx, desc)
	if err != nil {
		return ocispec.Index{}, err
	}

	var index ocispec.Index
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		return ocispec.Index{}, err
	}
	return index, nil
}

func getFilteredComponents(ctx context.Context, remote *oci.OrasRemote, manifest oci.Manifest, optionalComponents []string) ([]zarfTypes.ZarfComponent, error) {
	// get Zarf pkg from manifest
	var zarfPkg zarfTypes.ZarfPackage
	for _, desc := range manifest.Layers {
		if desc.Annotations[ocispec.AnnotationTitle] == config.ZarfYAML {
			zarfYAMLBytes, err := remote.FetchLayer(ctx, desc)
			if err != nil {
				return nil, err
			}
			if err := goyaml.Unmarshal(zarfYAMLBytes, &zarfPkg); err != nil {
				return nil, err
			}
			break
		}
	}

	// create filter for optional components and filter the pkg
	createFilter := filters.Combine(
		filters.ForDeploy(strings.Join(optionalComponents, ","), false),
	)
	filteredComponents, err := createFilter.Apply(zarfPkg)
	if err != nil {
		return nil, err
	}
	return filteredComponents, nil
}
