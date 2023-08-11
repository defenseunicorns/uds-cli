// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler contains functions for interacting with, managing and deploying UDS packages
package bundler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	goyaml "github.com/goccy/go-yaml"
	"github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"io"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
	"os"
	"path/filepath"
	"strings"
)

// Bundle creates the bundle and outputs to a local tarball
func Bundle(b *Bundler, signature []byte) error {
	if b.bundle.Metadata.Architecture == "" {
		return fmt.Errorf("architecture is required for bundling")
	}
	bundle := &b.bundle
	tmpDir := b.tmp
	ctx := context.TODO()
	message.Debug("Bundling", bundle.Metadata.Name, "to", tmpDir)
	store, err := ocistore.NewWithContext(context.TODO(), tmpDir)

	artifactPathMap := make(PathMap)

	// create root manifest for OCI artifact, will populate with refs to uds-bundle.yaml and zarf.yamls
	rootManifest := ocispec.Manifest{}
	rootManifest.MediaType = ocispec.MediaTypeImageManifest

	// push uds-bundle.yaml to OCI store
	bundleYamlDesc, err := pushBundleYamlToStore(ctx, store, bundle)
	if err != nil {
		return err
	}

	// append uds-bundle.yaml layer to rootManifest and grab path for archiving
	rootManifest.Layers = append(rootManifest.Layers, bundleYamlDesc)
	digest := strings.Split(bundleYamlDesc.Digest.String(), "sha256:")[1]
	artifactPathMap[filepath.Join(tmpDir, blobsDir, digest)] = filepath.Join(blobsDir, digest)

	// grab all Zarf pkgs from OCI and put blobs in OCI store
	for _, pkg := range bundle.ZarfPackages {
		url := fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
		remote, err := oci.NewOrasRemote(url)
		if err != nil {
			return err
		}
		// fetch the root manifest for this zarf.yaml
		pkgRootManifest, err := remote.FetchRoot()
		if err != nil {
			return err
		}

		zarfYamlDesc, err := pushZarfYamlToStore(ctx, store, pkgRootManifest)
		if err != nil {
			return err
		}

		// append zarf.yaml layer to root manifest and grab path for archiving
		rootManifest.Layers = append(rootManifest.Layers, zarfYamlDesc)
		digest := strings.Split(zarfYamlDesc.Digest.String(), "sha256:")[1]
		artifactPathMap[filepath.Join(tmpDir, blobsDir, digest)] = filepath.Join(blobsDir, digest)

		message.Debugf("Pushed %s sub-manifest into %s: %s", url, tmpDir, message.JSONValue(zarfYamlDesc))

		// get only the layers that are required by the components
		layersToCopy, err := getZarfLayers(remote, pkg, pkgRootManifest)
		spinner := message.NewProgressSpinner("Fetching layers from %s", remote.Repo().Reference.Repository)

		// pull layers from remote and write to OCI artifact dir
		for _, layer := range layersToCopy {
			digest = strings.Split(layer.Digest.String(), "sha256:")[1]
			filePath := filepath.Join(tmpDir, blobsDir, digest)
			artifactPathMap[filepath.Join(tmpDir, blobsDir, digest)] = filepath.Join(blobsDir, digest)

			if _, err := os.Stat(filePath); err == nil {
				continue
			}
			spinner.Updatef("Fetching %s", layer.Digest.Encoded())
			layerBytes, err := remote.FetchLayer(layer)
			if err != nil {
				return err
			}
			layerDesc := content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, layerBytes)
			err = store.Push(ctx, layerDesc, bytes.NewReader(layerBytes))
			if err != nil {
				return err
			}
		}
	}

	// create and push bundle manifest config
	manifestConfigDesc, err := createManifestConfig(bundle.Metadata, bundle.Build)
	if err != nil {
		return err
	}
	rootManifest.Config = manifestConfigDesc
	rootManifest.SchemaVersion = 2
	rootManifest.Annotations = manifestAnnotationsFromMetadata(&bundle.Metadata) // maps to registry UI
	manifestBytes, err := json.Marshal(rootManifest)
	if err != nil {
		return err
	}
	manifestDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestBytes)
	err = store.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes))

	// build index.json
	digest = strings.Split(manifestDesc.Digest.String(), "sha256:")[1]
	artifactPathMap[filepath.Join(tmpDir, blobsDir, digest)] = filepath.Join(blobsDir, digest)
	artifactPathMap[filepath.Join(tmpDir, "index.json")] = "index.json"

	// grab oci-layout
	artifactPathMap[filepath.Join(tmpDir, "oci-layout")] = "oci-layout"

	// push the bundle's signature todo: need to understand functionality and add tests
	if len(signature) > 0 {
		signatureDesc, err := pushBundleSignature(ctx, store, signature)
		if err != nil {
			return err
		}
		rootManifest.Layers = append(rootManifest.Layers, signatureDesc)
		message.Debug("Pushed", BundleYAMLSignature+":", message.JSONValue(signatureDesc))
	}

	// tarball the bundle
	writeTarball(bundle, artifactPathMap)
	if err != nil {
		return err
	}

	return nil
}

// BundleAndPublish creates the bundle in an OCI registry publishes w/ optional signature to the remote repository.
func BundleAndPublish(r *oci.OrasRemote, bundle *types.UDSBundle, signature []byte) error {
	if bundle.Metadata.Architecture == "" {
		return fmt.Errorf("architecture is required for bundling")
	}
	ref := r.Repo().Reference
	message.Debug("Bundling", bundle.Metadata.Name, "to", ref)

	rootManifest := ocispec.Manifest{}

	for _, pkg := range bundle.ZarfPackages {
		url := fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
		remote, err := oci.NewOrasRemote(url)
		if err != nil {
			return err
		}
		pkgRef := remote.Repo().Reference
		// fetch the root manifest so we can push it into the bundle
		pkgRootManifest, err := remote.FetchRoot()
		if err != nil {
			return err
		}

		zarfYamlDesc, err := json.Marshal(pkgRootManifest)
		if err != nil {
			return err
		}
		// push the manifest into the bundle
		manifestDesc, err := r.PushLayer(zarfYamlDesc, oci.ZarfLayerMediaTypeBlob)
		if err != nil {
			return err
		}
		// hack the media type to be a manifest
		manifestDesc.MediaType = ocispec.MediaTypeImageManifest
		message.Debugf("Pushed %s sub-manifest into %s: %s", url, ref, message.JSONValue(manifestDesc))
		rootManifest.Layers = append(rootManifest.Layers, manifestDesc)

		// get only the layers that are required by the components
		layersToCopy, err := getZarfLayers(remote, pkg, pkgRootManifest)

		// stream copy if different registry
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
			// blob mount if same registry
			message.Debugf("Performing a cross repository blob mount on %s from %s --> %s", ref, ref.Repository, ref.Repository)
			spinner := message.NewProgressSpinner("Mounting layers from %s", pkgRef.Repository)
			layersToCopy = append(layersToCopy, pkgRootManifest.Config) // why do we need root.Config in this case?
			for _, layer := range layersToCopy {
				spinner.Updatef("Mounting %s", layer.Digest.Encoded())
				// layer is the descriptor!! Verbiage "fetch" or "pull" refers to the actual layers
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
	rootManifest.Layers = append(rootManifest.Layers, bundleYamlDesc)

	// push the bundle's signature
	if len(signature) > 0 {
		bundleYamlSigDesc, err := r.PushLayer(signature, oci.ZarfLayerMediaTypeBlob)
		if err != nil {
			return err
		}
		bundleYamlSigDesc.Annotations = map[string]string{
			ocispec.AnnotationTitle: BundleYAMLSignature,
		}
		rootManifest.Layers = append(rootManifest.Layers, bundleYamlSigDesc)
		message.Debug("Pushed", BundleYAMLSignature+":", message.JSONValue(bundleYamlSigDesc))
	}

	// push the bundle manifest config
	configDesc, err := pushManifestConfigFromMetadata(r, &bundle.Metadata, &bundle.Build)
	if err != nil {
		return err
	}

	message.Debug("Pushed config:", message.JSONValue(configDesc))

	rootManifest.Config = configDesc

	rootManifest.SchemaVersion = 2

	rootManifest.Annotations = manifestAnnotationsFromMetadata(&bundle.Metadata) // maps to registry UI
	b, err := json.Marshal(rootManifest)
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

// pushBundleYamlToStore pushes the uds-bundle.yaml to a provided OCI store
func pushBundleYamlToStore(ctx context.Context, store *ocistore.Store, bundle *types.UDSBundle) (ocispec.Descriptor, error) {
	bundleYamlBytes, err := goyaml.Marshal(bundle)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	bundleYamlDesc := content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, bundleYamlBytes)
	bundleYamlDesc.Annotations = map[string]string{
		ocispec.AnnotationTitle: BundleYAML,
	}
	err = store.Push(ctx, bundleYamlDesc, bytes.NewReader(bundleYamlBytes))
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	message.Debug("Pushed", BundleYAML+":", message.JSONValue(bundleYamlDesc))
	return bundleYamlDesc, err
}

// pushZarfYamlToStore pushes a zarf.yaml to a provided OCI store
func pushZarfYamlToStore(ctx context.Context, store *ocistore.Store, manifest *oci.ZarfOCIManifest) (ocispec.Descriptor, error) {
	pkgManifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	zarfYamlDesc := content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, pkgManifestBytes)
	err = store.Push(ctx, zarfYamlDesc, bytes.NewReader(pkgManifestBytes))
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return zarfYamlDesc, err
}

// createManifestConfig creates a manifest config based on the uds-bundle.yaml
func createManifestConfig(metadata types.UDSMetadata, build types.UDSBuildData) (ocispec.Descriptor, error) {
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
	manifestConfigDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestConfigBytes)
	return manifestConfigDesc, err
}

// getZarfLayers grabs the necessary Zarf pkg layers from a remote OCI registry
func getZarfLayers(remote *oci.OrasRemote, pkg types.ZarfPackageImport, pkgRootManifest *oci.ZarfOCIManifest) ([]ocispec.Descriptor, error) {
	layersFromComponents, err := remote.LayersFromRequestedComponents(pkg.OptionalComponents)
	if err != nil {
		return nil, err
	}

	// get the layers that are always pulled
	var metadataLayers []ocispec.Descriptor
	for _, path := range oci.PackageAlwaysPull {
		layer := pkgRootManifest.Locate(path)
		if !oci.IsEmptyDescriptor(layer) {
			metadataLayers = append(metadataLayers, layer)
		}
	}
	layersToCopy := append(layersFromComponents, metadataLayers...)
	layersToCopy = append(layersToCopy, pkgRootManifest.Config)
	return layersToCopy, err
}

// writeTarball builds and writes a bundle tarball to disk based on a file map
func writeTarball(bundle *types.UDSBundle, artifactPathMap PathMap) error {
	format := archiver.CompressedArchive{
		Compression: archiver.Zstd{},
		Archival:    archiver.Tar{},
	}
	filename := fmt.Sprintf("%s%s-%s-%s.tar.zst", BundlePrefix, bundle.Metadata.Name, bundle.Metadata.Architecture, bundle.Metadata.Version)
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	dst := filepath.Join(cwd, filename)

	_ = os.RemoveAll(dst)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	files, err := archiver.FilesFromDisk(nil, artifactPathMap)
	if err != nil {
		return err
	}
	if err := format.Archive(context.TODO(), out, files); err != nil {
		return err
	}
	message.Infof("Bundle tarball saved to %s", dst)
	return nil
}

func pushBundleSignature(ctx context.Context, store *ocistore.Store, signature []byte) (ocispec.Descriptor, error) {
	signatureDesc := content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, signature)
	err := store.Push(ctx, signatureDesc, bytes.NewReader(signature))
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	signatureDesc.Annotations = map[string]string{
		ocispec.AnnotationTitle: BundleYAMLSignature,
	}
	return signatureDesc, err
}
