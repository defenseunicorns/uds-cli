// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler defines behavior for bundling packages
package bundler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler/fetcher"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	"github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
)

// LocalBundleOpts are the options for creating a local bundle
type LocalBundleOpts struct {
	Bundle    *types.UDSBundle
	TmpDstDir string
	SourceDir string
}

// LocalBundle enables create ops with local bundles
type LocalBundle struct {
	bundle    *types.UDSBundle
	tmpDstDir string
	sourceDir string
}

// NewLocalBundle creates a new local bundle
func NewLocalBundle(opts *LocalBundleOpts) *LocalBundle {
	return &LocalBundle{
		bundle:    opts.Bundle,
		tmpDstDir: opts.TmpDstDir,
		sourceDir: opts.SourceDir,
	}
}

// create creates the bundle and outputs to a local tarball
func (lo *LocalBundle) create(signature []byte, dev bool) error {
	bundle := lo.bundle
	if bundle.Metadata.Architecture == "" {
		return fmt.Errorf("architecture is required for bundling")
	}
	store, err := ocistore.NewWithContext(context.TODO(), lo.tmpDstDir)
	ctx := context.TODO()

	message.HeaderInfof("🐕 Fetching Packages")

	// create root manifest for bundle, will populate with refs to uds-bundle.yaml and zarf image manifests
	rootManifest := ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
	}

	fetcherConfig := fetcher.Config{
		Bundle:             bundle,
		Store:              store,
		TmpDstDir:          lo.tmpDstDir,
		NumPkgs:            len(lo.bundle.Packages),
		BundleRootManifest: &rootManifest,
	}

	message.Debug("Bundling", bundle.Metadata.Name, "to", lo.tmpDstDir)
	if err != nil {
		return err
	}

	artifactPathMap := make(types.PathMap)

	// grab all Zarf pkgs from OCI and put blobs in OCI store
	for i, pkg := range bundle.Packages {
		fetcherConfig.PkgIter = i
		pkgFetcher, err := fetcher.NewPkgFetcher(pkg, fetcherConfig)
		if err != nil {
			return err
		}
		layerDescs, err := pkgFetcher.Fetch()
		if err != nil {
			return err
		}

		// get the image manifest digest for the current package
		imageManifestDigest := strings.Split(bundle.Packages[i].Ref, ":")[1]

		// add to artifactPathMap for local tarball
		// todo: if we know the path to where the blobs are stored, we can use that instead of the artifactPathMap?
		var zarfYamlLayerSize int64
		var imageManifestPath string
		for _, layer := range layerDescs {
			digest := layer.Digest.Encoded()
			path := filepath.Join(lo.tmpDstDir, config.BlobsDir, digest)
			artifactPathMap[path] = filepath.Join(config.BlobsDir, digest)
			// if running in dev mode update zarf.yaml with YOLO and strip out images and repos
			if dev {
				if layer.Annotations["org.opencontainers.image.title"] == "zarf.yaml" {
					zarfYamlLayerSize = updateDevZarfYaml(path)
				} else if digest == imageManifestDigest {
					imageManifestPath = path
				}
			}
		}

		// if running in dev mode update zarf.yaml layer size in image manifest
		if dev {
			updateDevImageManifest(imageManifestPath, zarfYamlLayerSize)
		}
	}

	message.HeaderInfof("🚧 Building Bundle")

	// push uds-bundle.yaml to OCI store
	bundleYAMLDesc, err := pushBundleYAMLToStore(store, bundle)
	if err != nil {
		return err
	}

	// append uds-bundle.yaml layer to rootManifest and grab path for archiving
	rootManifest.Layers = append(rootManifest.Layers, bundleYAMLDesc)
	digest := bundleYAMLDesc.Digest.Encoded()
	artifactPathMap[filepath.Join(lo.tmpDstDir, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)

	// create and push bundle manifest config
	manifestConfigDesc, err := pushManifestConfig(store, bundle.Metadata, bundle.Build)
	if err != nil {
		return err
	}
	manifestConfigDigest := manifestConfigDesc.Digest.Encoded()
	artifactPathMap[filepath.Join(lo.tmpDstDir, config.BlobsDir, manifestConfigDigest)] = filepath.Join(config.BlobsDir, manifestConfigDigest)

	rootManifest.Config = manifestConfigDesc
	rootManifest.SchemaVersion = 2
	rootManifest.Annotations = manifestAnnotationsFromMetadata(&bundle.Metadata) // maps to registry UI
	rootManifestDesc, err := utils.ToOCIStore(rootManifest, ocispec.MediaTypeImageManifest, store)
	if err != nil {
		return err
	}
	digest = rootManifestDesc.Digest.Encoded()
	artifactPathMap[filepath.Join(lo.tmpDstDir, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)

	// grab index.json
	artifactPathMap[filepath.Join(lo.tmpDstDir, "index.json")] = "index.json"

	// grab oci-layout
	artifactPathMap[filepath.Join(lo.tmpDstDir, "oci-layout")] = "oci-layout"

	// push the bundle's signature todo: need to understand functionality and add tests
	if len(signature) > 0 {
		signatureDesc, err := pushBundleSignature(store, signature)
		if err != nil {
			return err
		}
		rootManifest.Layers = append(rootManifest.Layers, signatureDesc)
		message.Debug("Pushed", config.BundleYAMLSignature+":", message.JSONValue(signatureDesc))
	}

	// tag the local bundle artifact
	// todo: no need to tag the local artifact
	err = store.Tag(ctx, rootManifestDesc, bundle.Metadata.Version)
	if err != nil {
		return err
	}
	// ensure the bundle root manifest is the only manifest in the index.json
	err = cleanIndexJSON(lo.tmpDstDir, rootManifestDesc)
	if err != nil {
		return err
	}

	// tarball the bundle
	err = writeTarball(bundle, artifactPathMap, lo.sourceDir, dev)
	if err != nil {
		return err
	}

	return nil
}

// pushBundleYAMLToStore pushes the uds-bundle.yaml to a provided OCI store
func pushBundleYAMLToStore(store *ocistore.Store, bundle *types.UDSBundle) (ocispec.Descriptor, error) {
	ctx := context.TODO()
	bundleYAMLBytes, err := goyaml.Marshal(bundle)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	bundleYamlDesc := content.NewDescriptorFromBytes(zoci.ZarfLayerMediaTypeBlob, bundleYAMLBytes)
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
	manifestConfigDesc, err := utils.ToOCIStore(manifestConfig, zoci.ZarfLayerMediaTypeBlob, store)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	return manifestConfigDesc, err
}

// writeTarball builds and writes a bundle tarball to disk based on a file map
func writeTarball(bundle *types.UDSBundle, artifactPathMap types.PathMap, sourceDir string, dev bool) error {
	format := archiver.CompressedArchive{
		Compression: archiver.Zstd{},
		Archival:    archiver.Tar{},
	}

	var filename string
	if dev {
		filename = fmt.Sprintf("%s%s-%s-%s.tar.zst", config.DevBundlePrefix, bundle.Metadata.Name, bundle.Metadata.Architecture, bundle.Metadata.Version)
	} else {
		filename = fmt.Sprintf("%s%s-%s-%s.tar.zst", config.BundlePrefix, bundle.Metadata.Name, bundle.Metadata.Architecture, bundle.Metadata.Version)
	}

	dst := filepath.Join(sourceDir, filename)

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

func pushBundleSignature(store *ocistore.Store, signature []byte) (ocispec.Descriptor, error) {
	ctx := context.TODO()
	signatureDesc := content.NewDescriptorFromBytes(zoci.ZarfLayerMediaTypeBlob, signature)
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
func cleanIndexJSON(tmpDir string, bundleRootDesc ocispec.Descriptor) error {
	indexBytes, err := os.ReadFile(filepath.Join(tmpDir, "index.json"))
	if err != nil {
		return err
	}
	var index ocispec.Index
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		return err
	}

	for _, manifestDesc := range index.Manifests {
		if manifestDesc.Digest.Encoded() == bundleRootDesc.Digest.Encoded() {
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

// updateDevZarfYaml updates zarf.yaml with YOLO and strips out images and repos
func updateDevZarfYaml(path string) int64 {

	// Read YAML into struct
	var zarfYamlConfig zarfTypes.ZarfPackage
	err := zarfUtils.ReadYaml(path, &zarfYamlConfig)
	if err != nil {
		message.Fatalf(err, "Failed to read zarf.yaml: %s", err.Error())
	}

	// Add the yolo to the metadata
	zarfYamlConfig.Metadata.YOLO = true

	// strip out all images and repos
	for idx := range zarfYamlConfig.Components {
		zarfYamlConfig.Components[idx].Images = []string{}
		zarfYamlConfig.Components[idx].Repos = []string{}
	}

	// delete the old zarf yaml
	err = os.Remove(path)
	if err != nil {
		message.Fatalf(err, "Failed to delete zarf.yaml: %s", err.Error())
	}
	// Write zarf yaml
	err = zarfUtils.WriteYaml(path, &zarfYamlConfig, 0444)
	if err != nil {
		message.Fatalf(err, "Failed to update zarf.yaml: %s", err.Error())
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		message.Fatalf(err, "Failed to stat zarf.yaml: %s", err.Error())
	}

	return fileInfo.Size()
}

// updateDevImageManifest updates the image manifest with the new zarf.yaml layer size
func updateDevImageManifest(imageManifestPath string, zarfYamlLayerSize int64) {
	// Read imageManifest into struct
	indexBytes, err := os.ReadFile(imageManifestPath)
	if err != nil {
		message.Fatalf(err, "Failed to read image manifest %s: %s", imageManifestPath, err.Error())
	}
	var imageManifest ocispec.Manifest
	if err := json.Unmarshal(indexBytes, &imageManifest); err != nil {
		message.Fatalf(err, "Failed to unmarshal image manifest: %s", err.Error())
	}

	for idx := range imageManifest.Layers {
		if imageManifest.Layers[idx].Annotations["org.opencontainers.image.title"] == "zarf.yaml" {
			imageManifest.Layers[idx].Size = zarfYamlLayerSize
		}
	}

	// delete the old image manifest
	_ = os.Remove(imageManifestPath)

	// Write image manifest with updated size
	err = utils.ToLocalFile(&imageManifest, imageManifestPath)
	if err != nil {
		message.Fatalf(err, "Failed to write image manifest %s: %s", imageManifestPath, err.Error())
	}
}
