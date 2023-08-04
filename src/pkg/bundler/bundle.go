// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler contains functions for interacting with, managing and deploying UDS packages
package bundler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	goyaml "github.com/goccy/go-yaml"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
)

// Bundle publishes the given bundle w/ optional signature to the remote repository.
func Bundle(r *oci.OrasRemote, bundle *types.UDSBundle, signature []byte) error {
	if bundle.Metadata.Architecture == "" {
		return fmt.Errorf("architecture is required for bundling")
	}
	ref := r.Repo().Reference
	message.Debug("Bundling", bundle.Metadata.Name, "to", ref)

	manifest := ocispec.Manifest{}

	for _, pkg := range bundle.ZarfPackages {
		url := fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
		remote, err := oci.NewOrasRemote(url)
		if err != nil {
			return err
		}
		pkgRef := remote.Repo().Reference
		// fetch the root manifest so we can push it into the bundle
		root, err := remote.FetchRoot()
		if err != nil {
			return err
		}
		manifestBytes, err := json.Marshal(root)
		if err != nil {
			return err
		}
		// push the manifest into the bundle
		manifestDesc, err := r.PushLayer(manifestBytes, oci.ZarfLayerMediaTypeBlob)
		if err != nil {
			return err
		}
		// hack the media type to be a manifest
		manifestDesc.MediaType = ocispec.MediaTypeImageManifest
		message.Debugf("Pushed %s sub-manifest into %s: %s", url, ref, message.JSONValue(manifestDesc))
		manifest.Layers = append(manifest.Layers, manifestDesc)

		// get only the layers that are required by the components
		layersFromComponents, err := remote.LayersFromRequestedComponents(pkg.OptionalComponents)
		if err != nil {
			return err
		}

		// get the layers that are always pulled
		metadataLayers := []ocispec.Descriptor{}
		for _, path := range oci.PackageAlwaysPull {
			layer := root.Locate(path)
			if !oci.IsEmptyDescriptor(layer) {
				metadataLayers = append(metadataLayers, layer)
			}
		}

		layersToCopy := append(layersFromComponents, metadataLayers...)

		// stream copy the blobs, otherwise do a blob mount
		if remote.Repo().Reference.Registry != ref.Registry {
			message.Debugf("Streaming layers from %s --> %s", pkgRef, ref)

			// filterLayers returns true if the layer is in the list of layers to copy, this allows for
			// copying only the layers that are required by the required + specified optional components
			filterLayers := func(d ocispec.Descriptor) bool {
				for _, layer := range layersToCopy {
					if layer.Digest == d.Digest {
						return true
					}
				}
				return false
			}

			if err := oci.CopyPackage(remote, r, filterLayers, config.CommonOptions.OCIConcurrency); err != nil {
				return err
			}
		} else {
			message.Debugf("Performing a cross repository blob mount on %s from %s --> %s", ref, ref.Repository, ref.Repository)
			spinner := message.NewProgressSpinner("Mounting layers from %s", pkgRef.Repository)
			layersToCopy = append(layersToCopy, root.Config)

			for _, layer := range layersToCopy {
				spinner.Updatef("Mounting %s", layer.Digest.Encoded())
				if err := r.Repo().Mount(context.TODO(), layer, pkgRef.Repository, func() (io.ReadCloser, error) {
					return remote.Repo().Fetch(context.TODO(), layer)
				}); err != nil {
					return err
				}
			}

			spinner.Successf("Mounted %d layers", len(layersToCopy))
		}
	}

	// push the bundle's metadata
	bundleYamlBytes, err := goyaml.Marshal(bundle)
	if err != nil {
		return err
	}
	bundleYamlDesc, err := r.PushLayer(bundleYamlBytes, oci.ZarfLayerMediaTypeBlob)
	if err != nil {
		return err
	}
	bundleYamlDesc.Annotations = map[string]string{
		ocispec.AnnotationTitle: BundleYAML,
	}

	message.Debug("Pushed", BundleYAML+":", message.JSONValue(bundleYamlDesc))
	manifest.Layers = append(manifest.Layers, bundleYamlDesc)

	// push the bundle's signature
	if len(signature) > 0 {
		bundleYamlSigDesc, err := r.PushLayer(signature, oci.ZarfLayerMediaTypeBlob)
		if err != nil {
			return err
		}
		bundleYamlSigDesc.Annotations = map[string]string{
			ocispec.AnnotationTitle: BundleYAMLSignature,
		}
		manifest.Layers = append(manifest.Layers, bundleYamlSigDesc)
		message.Debug("Pushed", BundleYAMLSignature+":", message.JSONValue(bundleYamlSigDesc))
	}

	// push the bundle manifest config
	configDesc, err := pushManifestConfigFromMetadata(r, &bundle.Metadata, &bundle.Build)
	if err != nil {
		return err
	}

	message.Debug("Pushed config:", message.JSONValue(configDesc))

	manifest.Config = configDesc

	manifest.SchemaVersion = 2

	manifest.Annotations = manifestAnnotationsFromMetadata(&bundle.Metadata)
	b, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	expected := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, b)

	message.Debug("Pushing manifest:", message.JSONValue(expected))

	if err := r.Repo().Manifests().PushReference(context.TODO(), expected, bytes.NewReader(b), ref.Reference); err != nil {
		return fmt.Errorf("failed to push manifest: %w", err)
	}

	message.Successf("Published %s [%s]", ref, expected.MediaType)

	message.HorizontalRule()
	flags := ""
	if config.CommonOptions.Insecure {
		flags = "--insecure"
	}
	message.Title("To inspect/deploy/pull:", "")
	message.Command("bundle inspect oci://%s %s", ref, flags)
	message.Command("bundle deploy oci://%s %s", ref, flags)
	message.Command("bundle pull oci://%s %s", ref, flags)

	return nil
}

// copied from: https://github.com/defenseunicorns/zarf/blob/main/src/pkg/oci/push.go
func pushManifestConfigFromMetadata(r *oci.OrasRemote, metadata *types.UDSMetadata, build *types.UDSBuildData) (ocispec.Descriptor, error) {
	annotations := map[string]string{
		ocispec.AnnotationTitle:       metadata.Name,
		ocispec.AnnotationDescription: metadata.Description,
	}
	manifestConfig := oci.ConfigPartial{
		Architecture: build.Architecture,
		OCIVersion:   "1.0.1",
		Annotations:  annotations,
	}
	manifestConfigBytes, err := json.Marshal(manifestConfig)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return r.PushLayer(manifestConfigBytes, ocispec.MediaTypeImageConfig)
}

// copied from: https://github.com/defenseunicorns/zarf/blob/main/src/pkg/oci/push.go
func manifestAnnotationsFromMetadata(metadata *types.UDSMetadata) map[string]string {
	annotations := map[string]string{
		ocispec.AnnotationDescription: metadata.Description,
	}

	if url := metadata.URL; url != "" {
		annotations[ocispec.AnnotationURL] = url
	}
	if authors := metadata.Authors; authors != "" {
		annotations[ocispec.AnnotationAuthors] = authors
	}
	if documentation := metadata.Documentation; documentation != "" {
		annotations[ocispec.AnnotationDocumentation] = documentation
	}
	if source := metadata.Source; source != "" {
		annotations[ocispec.AnnotationSource] = source
	}
	if vendor := metadata.Vendor; vendor != "" {
		annotations[ocispec.AnnotationVendor] = vendor
	}

	return annotations
}
