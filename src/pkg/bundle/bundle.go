// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	goyaml "github.com/goccy/go-yaml"
	"github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
)

// Create creates the bundle and outputs to a local tarball
func Create(b *Bundler, signature []byte) error {
	message.HeaderInfof("🐕 Fetching Packages")

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
	rootManifest := ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
	}

	// grab all Zarf pkgs from OCI and put blobs in OCI store
	for i, pkg := range bundle.Packages {
		fetchSpinner := message.NewProgressSpinner("Fetching package %s", pkg.Name)
		zarfPackageName := ""
		zarfLayerAdded := false
		defer fetchSpinner.Stop()

		if pkg.Repository != "" {
			url := fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
			remoteBundler, err := bundler.NewRemoteBundler(pkg, url, store, nil, b.tmp)
			if err != nil {
				return err
			}

			layerDescs, err := remoteBundler.LayersToBundle(fetchSpinner, i+1, len(bundle.Packages))
			if err != nil {
				return err
			}

			// grab layers for archiving
			for i, layerDesc := range layerDescs {
				if layerDesc.MediaType == ocispec.MediaTypeImageManifest {
					// rewrite the Zarf image manifest to have media type of Zarf blob
					err = os.Remove(filepath.Join(b.tmp, config.BlobsDir, layerDesc.Digest.Encoded()))
					if err != nil {
						return err
					}
					err = utils.FetchLayerAndStore(layerDesc, remoteBundler.RemoteSrc, store)
					if err != nil {
						return err
					}
					// ensure media type is Zarf blob for layers in the bundle's root manifest
					layerDesc.MediaType = oci.ZarfLayerMediaTypeBlob

					// add package name annotations
					annotations := make(map[string]string)
					layerDesc.Annotations = annotations
					layerDesc.Annotations[config.UDSPackageNameAnnotation] = pkg.Name

					// If zarf package name has been obtained from zarf config, set the zarf package name annotation
					if zarfPackageName != "" {
						layerDesc.Annotations[config.ZarfPackageNameAnnotation] = zarfPackageName
					}

					rootManifest.Layers = append(rootManifest.Layers, layerDesc)
					zarfLayerAdded = true
				}
				if layerDesc.MediaType == "application/vnd.zarf.config.v1+json" {
					// read in and unmarshall zarf config
					jsonData, err := os.ReadFile(filepath.Join(b.tmp, config.BlobsDir, layerDesc.Digest.Encoded()))
					if err != nil {
						return err
					}
					var data map[string]interface{}
					err = json.Unmarshal(jsonData, &data)
					if err != nil {
						return err
					}
					zarfPackageName = data["annotations"].(map[string]interface{})["org.opencontainers.image.title"].(string)
					// Check if zarf image manifest has been added to root manifest already, if so add zarfPackageName annotation
					if zarfLayerAdded {
						rootManifest.Layers = append(rootManifest.Layers, layerDesc)
						rootManifest.Layers[i].Annotations[config.ZarfPackageNameAnnotation] = zarfPackageName
					}
				}
				digest := layerDesc.Digest.Encoded()
				artifactPathMap[filepath.Join(b.tmp, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)
			}
		} else if pkg.Path != "" {
			pkgTmp, err := zarfUtils.MakeTempDir("")
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

			// add package name annotations, for local zarf packages, these names will be the same
			zarfPkgDesc.Annotations = make(map[string]string)
			zarfPkgDesc.Annotations[config.UDSPackageNameAnnotation] = pkg.Name
			zarfPkgDesc.Annotations[config.ZarfPackageNameAnnotation] = pkg.Name

			if err != nil {
				return err
			}

			// put digest in uds-bundle.yaml to reference during deploy
			bundle.Packages[i].Ref = bundle.Packages[i].Ref + "-" + bundle.Metadata.Architecture + "@sha256:" + zarfPkgDesc.Digest.Encoded()

			// append zarf image manifest to bundle root manifest and grab path for archiving
			rootManifest.Layers = append(rootManifest.Layers, zarfPkgDesc)
			digest := zarfPkgDesc.Digest.Encoded()
			artifactPathMap[filepath.Join(b.tmp, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)

		} else {
			return fmt.Errorf("todo: haven't we already validated that Path or Repository is valid")
		}

		fetchSpinner.Successf("Fetched package: %s", pkg.Name)
	}

	message.HeaderInfof("🚧 Building Bundle")

	// push uds-bundle.yaml to OCI store
	bundleYAMLDesc, err := pushBundleYAMLToStore(ctx, store, bundle)
	if err != nil {
		return err
	}

	// append uds-bundle.yaml layer to rootManifest and grab path for archiving
	rootManifest.Layers = append(rootManifest.Layers, bundleYAMLDesc)
	digest := bundleYAMLDesc.Digest.Encoded()
	artifactPathMap[filepath.Join(b.tmp, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)

	// create and push bundle manifest config
	manifestConfigDesc, err := pushManifestConfig(store, bundle.Metadata, bundle.Build)
	if err != nil {
		return err
	}
	manifestConfigDigest := manifestConfigDesc.Digest.Encoded()
	artifactPathMap[filepath.Join(b.tmp, config.BlobsDir, manifestConfigDigest)] = filepath.Join(config.BlobsDir, manifestConfigDigest)

	rootManifest.Config = manifestConfigDesc
	rootManifest.SchemaVersion = 2
	rootManifest.Annotations = manifestAnnotationsFromMetadata(&bundle.Metadata) // maps to registry UI
	rootManifestDesc, err := utils.ToOCIStore(rootManifest, ocispec.MediaTypeImageManifest, store)
	if err != nil {
		return err
	}
	digest = rootManifestDesc.Digest.Encoded()
	artifactPathMap[filepath.Join(b.tmp, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)

	// grab index.json
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
		message.Debug("Pushed", config.BundleYAMLSignature+":", message.JSONValue(signatureDesc))
	}

	// tag the local bundle artifact
	ref := fmt.Sprintf("%s-%s", bundle.Metadata.Version, bundle.Metadata.Architecture)
	err = store.Tag(ctx, rootManifestDesc, ref)
	if err != nil {
		return err
	}
	err = cleanIndexJSON(b.tmp, ref)
	if err != nil {
		return err
	}

	// tarball the bundle
	err = writeTarball(bundle, artifactPathMap)
	if err != nil {
		return err
	}

	return nil
}

// CreateAndPublish creates the bundle in an OCI registry publishes w/ optional signature to the remote repository.
func CreateAndPublish(remoteDst *oci.OrasRemote, bundle *types.UDSBundle, signature []byte) error {
	if bundle.Metadata.Architecture == "" {
		return fmt.Errorf("architecture is required for bundling")
	}
	dstRef := remoteDst.Repo().Reference
	message.Debug("Bundling", bundle.Metadata.Name, "to", dstRef)

	rootManifest := ocispec.Manifest{}

	for i, pkg := range bundle.Packages {
		url := fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
		remoteBundler, err := bundler.NewRemoteBundler(pkg, url, nil, remoteDst, "")
		if err != nil {
			return err
		}

		zarfManifestDesc, err := remoteBundler.PushManifest()
		if err != nil {
			return err
		}

		// ensure media type is a Zarf blob and append to bundle root manifest
		zarfManifestDesc.MediaType = oci.ZarfLayerMediaTypeBlob
		message.Debugf("Pushed %s sub-manifest into %s: %s", url, dstRef, message.JSONValue(zarfManifestDesc))

		// add package name annotations to zarf manifest
		zarfYamlPackage, err := remoteBundler.RemoteSrc.FetchZarfYAML()
		if err != nil {
			return err
		}
		zarfManifestDesc.Annotations = make(map[string]string)
		zarfManifestDesc.Annotations[config.UDSPackageNameAnnotation] = pkg.Name
		zarfManifestDesc.Annotations[config.ZarfPackageNameAnnotation] = zarfYamlPackage.Metadata.Name

		rootManifest.Layers = append(rootManifest.Layers, zarfManifestDesc)

		pushSpinner := message.NewProgressSpinner("")

		defer pushSpinner.Stop()

		_, err = remoteBundler.LayersToBundle(pushSpinner, i+1, len(bundle.Packages))
		if err != nil {
			return err
		}

		pushSpinner.Successf("Pushed package: %s", pkg.Name)
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
			ocispec.AnnotationTitle: config.BundleYAMLSignature,
		}
		rootManifest.Layers = append(rootManifest.Layers, bundleYamlSigDesc)
		message.Debug("Pushed", config.BundleYAMLSignature+":", message.JSONValue(bundleYamlSigDesc))
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

	_, err = utils.ToOCIRemote(rootManifest, ocispec.MediaTypeImageManifest, remoteDst)
	if err != nil {
		return err
	}

	message.HorizontalRule()
	flags := ""
	if config.CommonOptions.Insecure {
		flags = "--insecure"
	}
	message.Title("To inspect/deploy/pull:", "")
	message.Command("inspect oci://%s %s", dstRef, flags)
	message.Command("deploy oci://%s %s", dstRef, flags)
	message.Command("pull oci://%s %s", dstRef, flags)

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
	manifestConfigDesc, err := utils.ToOCIRemote(manifestConfig, oci.ZarfLayerMediaTypeBlob, r)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return manifestConfigDesc, nil
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

// pushBundleYAMLToStore pushes the uds-bundle.yaml to a provided OCI store
func pushBundleYAMLToStore(ctx context.Context, store *ocistore.Store, bundle *types.UDSBundle) (ocispec.Descriptor, error) {
	bundleYAMLBytes, err := goyaml.Marshal(bundle)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	bundleYamlDesc := content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, bundleYAMLBytes)
	bundleYamlDesc.Annotations = map[string]string{
		ocispec.AnnotationTitle: config.BundleYAML,
	}
	err = store.Push(ctx, bundleYamlDesc, bytes.NewReader(bundleYAMLBytes))
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	message.Debug("Pushed", config.BundleYAML+":", message.JSONValue(bundleYamlDesc))
	return bundleYamlDesc, err
}

// pushManifestConfig creates a manifest config based on the uds-bundle.yaml
func pushManifestConfig(store *ocistore.Store, metadata types.UDSMetadata, build types.UDSBuildData) (ocispec.Descriptor, error) {
	annotations := map[string]string{
		ocispec.AnnotationTitle:       metadata.Name,
		ocispec.AnnotationDescription: metadata.Description,
	}
	manifestConfig := oci.ConfigPartial{
		Architecture: build.Architecture,
		OCIVersion:   "1.0.1",
		Annotations:  annotations,
	}
	manifestConfigDesc, err := utils.ToOCIStore(manifestConfig, oci.ZarfLayerMediaTypeBlob, store)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	return manifestConfigDesc, err
}

// writeTarball builds and writes a bundle tarball to disk based on a file map
func writeTarball(bundle *types.UDSBundle, artifactPathMap PathMap) error {
	format := archiver.CompressedArchive{
		Compression: archiver.Zstd{},
		Archival:    archiver.Tar{},
	}
	filename := fmt.Sprintf("%s%s-%s-%s.tar.zst", config.BundlePrefix, bundle.Metadata.Name, bundle.Metadata.Architecture, bundle.Metadata.Version)
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

	archiveErrorChan := make(chan error, len(files))
	jobs := make(chan archiver.ArchiveAsyncJob, len(files))

	for _, file := range files {
		archiveJob := archiver.ArchiveAsyncJob{
			File:   file,
			Result: archiveErrorChan,
		}
		jobs <- archiveJob
	}

	close(jobs)

	archiveErrGroup, ctx := errgroup.WithContext(context.TODO())

	archiveBar := message.NewProgressBar(int64(len(jobs)), "Creating bundle archive")

	defer archiveBar.Stop()

	archiveErrGroup.Go(func() error {
		return format.ArchiveAsync(ctx, out, jobs)
	})

jobLoop:
	for len(jobs) != 0 {
		select {
		case err := <-archiveErrorChan:
			if err != nil {
				return err
			} else {
				archiveBar.Add(1)
			}
		case <-ctx.Done():
			break jobLoop
		}
	}

	if err := archiveErrGroup.Wait(); err != nil {
		return err
	}

	archiveBar.Successf("Created bundle archive at: %s", dst)
	return nil
}

func pushBundleSignature(ctx context.Context, store *ocistore.Store, signature []byte) (ocispec.Descriptor, error) {
	signatureDesc := content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, signature)
	err := store.Push(ctx, signatureDesc, bytes.NewReader(signature))
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	signatureDesc.Annotations = map[string]string{
		ocispec.AnnotationTitle: config.BundleYAMLSignature,
	}
	return signatureDesc, err
}

// rebuild index.json because copying remote Zarf pkgs adds unnecessary entries
// this is due to root manifest in Zarf packages having an image manifest media type
func cleanIndexJSON(tmpDir, ref string) error {
	indexBytes, err := os.ReadFile(filepath.Join(tmpDir, "index.json"))
	if err != nil {
		return err
	}
	var index ocispec.Index
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		return err
	}

	for _, manifestDesc := range index.Manifests {
		if manifestDesc.Annotations[ocispec.AnnotationRefName] == ref {
			index.Manifests = []ocispec.Descriptor{manifestDesc}
			break
		}
	}

	err = utils.ToLocalFile(index, filepath.Join(tmpDir, "index.json"))
	if err != nil {
		return err
	}
	return nil
}
