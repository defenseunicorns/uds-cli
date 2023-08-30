// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	goyaml "github.com/goccy/go-yaml"
	"github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
	"os"
	"path/filepath"
)

// Bundle creates the bundle and outputs to a local tarball
func Bundle(b *Bundler, signature []byte) error {
	if b.bundle.Metadata.Architecture == "" {
		return fmt.Errorf("architecture is required for bundling")
	}
	bundle := &b.bundle
	ctx := context.TODO()
	message.Debug("Bundling", bundle.Metadata.Name, "to", b.tmp)
	store, err := ocistore.NewWithContext(context.TODO(), b.tmp)
	if err != nil {
		return err
	}

	artifactPathMap := make(PathMap)

	// create root manifest for OCI artifact, will populate with refs to uds-bundle.yaml and zarf.yamls
	rootManifest := ocispec.Manifest{}
	rootManifest.MediaType = ocispec.MediaTypeImageManifest

	// grab all Zarf pkgs from OCI and put blobs in OCI store
	for i, pkg := range bundle.ZarfPackages {
		if pkg.Repository != "" {
			url := fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
			remoteBundler, err := bundler.NewRemoteBundler(pkg, url, store, nil)
			if err != nil {
				return err
			}

			zarfYamlDesc, err := remoteBundler.PushManifest()
			if err != nil {
				return err
			}

			// append zarf pkg manifest to root manifest and grab path for archiving
			rootManifest.Layers = append(rootManifest.Layers, zarfYamlDesc)
			digest := zarfYamlDesc.Digest.Encoded()
			artifactPathMap[filepath.Join(b.tmp, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)

			message.Debugf("Pushed %s sub-manifest into %s: %s", url, b.tmp, message.JSONValue(zarfYamlDesc))
			layerDescs, err := remoteBundler.PushLayers()
			if err != nil {
				return err
			}

			// grab layers for archiving
			for _, layerDesc := range layerDescs {
				digest = layerDesc.Digest.Encoded()
				artifactPathMap[filepath.Join(b.tmp, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)
			}
		} else if pkg.Path != "" {
			pkgTmp, err := utils.MakeTempDir()
			defer os.RemoveAll(pkgTmp)
			if err != nil {
				return err
			}

			localBundler := bundler.NewLocalBundler(pkg.Path, pkgTmp)
			if err != nil {
				return err
			}

			err = localBundler.Extract()
			if err != nil {
				return err
			}

			zarfPkg, err := localBundler.Load()
			if err != nil {
				return err
			}

			zarfPkgDesc, err := localBundler.ToBundle(store, zarfPkg, artifactPathMap, b.tmp, pkgTmp)
			if err != nil {
				return err
			}

			// put digest in uds-bundle.yaml to reference during deploy
			bundle.ZarfPackages[i].Ref = bundle.ZarfPackages[i].Ref + "-" + bundle.Metadata.Architecture + "@sha256:" + zarfPkgDesc.Digest.Encoded()

			// append zarf.yaml layer to root manifest and grab path for archiving
			rootManifest.Layers = append(rootManifest.Layers, zarfPkgDesc)
			digest := zarfPkgDesc.Digest.Encoded()
			artifactPathMap[filepath.Join(b.tmp, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)

		} else {
			return fmt.Errorf("todo: haven't we already validated that Path or Repository is valid")
		}
	}

	// push uds-bundle.yaml to OCI store
	bundleManifestDesc, err := pushBundleManifestToStore(ctx, store, bundle)
	if err != nil {
		return err
	}

	// append uds-bundle.yaml layer to rootManifest and grab path for archiving
	rootManifest.Layers = append(rootManifest.Layers, bundleManifestDesc)
	digest := bundleManifestDesc.Digest.Encoded()
	artifactPathMap[filepath.Join(b.tmp, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)

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
	if err := store.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil {
		return err
	}

	// build index.json
	digest = manifestDesc.Digest.Encoded()
	artifactPathMap[filepath.Join(b.tmp, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)
	artifactPathMap[filepath.Join(b.tmp, "index.json")] = "index.json"

	// grab oci-layout
	artifactPathMap[filepath.Join(b.tmp, "oci-layout")] = "oci-layout"

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
	err = writeTarball(bundle, artifactPathMap)
	if err != nil {
		return err
	}

	return nil
}

// BundleAndPublish creates the bundle in an OCI registry publishes w/ optional signature to the remote repository.
func BundleAndPublish(remoteDst *oci.OrasRemote, bundle *types.UDSBundle, signature []byte) error {
	if bundle.Metadata.Architecture == "" {
		return fmt.Errorf("architecture is required for bundling")
	}
	dstRef := remoteDst.Repo().Reference
	message.Debug("Bundling", bundle.Metadata.Name, "to", dstRef)

	rootManifest := ocispec.Manifest{}

	for _, pkg := range bundle.ZarfPackages {
		url := fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
		remoteBundler, err := bundler.NewRemoteBundler(pkg, url, nil, remoteDst)

		zarfManifestDesc, err := remoteBundler.PushManifest()
		if err != nil {
			return err
		}

		// hack the media type to be a manifest and append to bundle root manifest
		zarfManifestDesc.MediaType = ocispec.MediaTypeImageManifest
		message.Debugf("Pushed %s sub-manifest into %s: %s", url, dstRef, message.JSONValue(zarfManifestDesc))
		rootManifest.Layers = append(rootManifest.Layers, zarfManifestDesc)

		// hack the media type to be a manifest
		zarfManifestDesc.MediaType = ocispec.MediaTypeImageManifest
		message.Debugf("Pushed %s sub-manifest into %s: %s", url, dstRef, message.JSONValue(zarfManifestDesc))
		rootManifest.Layers = append(rootManifest.Layers, zarfManifestDesc)

		_, err = remoteBundler.PushLayers()
		if err != nil {
			return err
		}
	}

	// push the bundle's metadata
	bundleYamlBytes, err := goyaml.Marshal(bundle)
	if err != nil {
		return err
	}
	bundleYamlDesc, err := remoteDst.PushLayer(bundleYamlBytes, oci.ZarfLayerMediaTypeBlob)
	if err != nil {
		return err
	}
	bundleYamlDesc.Annotations = map[string]string{
		ocispec.AnnotationTitle: config.BundleYAML,
	}

	message.Debug("Pushed", config.BundleYAML+":", message.JSONValue(bundleYamlDesc))
	rootManifest.Layers = append(rootManifest.Layers, bundleYamlDesc)

	// push the bundle's signature
	if len(signature) > 0 {
		bundleYamlSigDesc, err := remoteDst.PushLayer(signature, oci.ZarfLayerMediaTypeBlob)
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
	configDesc, err := pushManifestConfigFromMetadata(remoteDst, &bundle.Metadata, &bundle.Build)
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

	if err := remoteDst.Repo().Manifests().PushReference(context.TODO(), expected, bytes.NewReader(b), dstRef.Reference); err != nil {
		return fmt.Errorf("failed to push manifest: %w", err)
	}

	message.Successf("Published %s [%s]", dstRef, expected.MediaType)

	message.HorizontalRule()
	flags := ""
	if config.CommonOptions.Insecure {
		flags = "--insecure"
	}
	message.Title("To inspect/deploy/pull:", "")
	message.Command("bundle inspect oci://%s %s", dstRef, flags)
	message.Command("bundle deploy oci://%s %s", dstRef, flags)
	message.Command("bundle pull oci://%s %s", dstRef, flags)

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

// pushBundleManifestToStore pushes the uds-bundle.yaml to a provided OCI store
func pushBundleManifestToStore(ctx context.Context, store *ocistore.Store, bundle *types.UDSBundle) (ocispec.Descriptor, error) {
	bundleManifestBytes, err := goyaml.Marshal(bundle)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	bundleYamlDesc := content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, bundleManifestBytes)
	bundleYamlDesc.Annotations = map[string]string{
		ocispec.AnnotationTitle: config.BundleYAML,
	}
	err = store.Push(ctx, bundleYamlDesc, bytes.NewReader(bundleManifestBytes))
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	message.Debug("Pushed", config.BundleYAML+":", message.JSONValue(bundleYamlDesc))
	return bundleYamlDesc, err
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
