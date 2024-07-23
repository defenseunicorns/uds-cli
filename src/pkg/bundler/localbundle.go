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

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler/fetcher"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/interactive"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	goyaml "github.com/goccy/go-yaml"
	"github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"

	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
)

// LocalBundleOpts are the options for creating a local bundle
type LocalBundleOpts struct {
	Bundle    *types.UDSBundle
	TmpDstDir string
	SourceDir string
	OutputDir string
}

// LocalBundle enables create ops with local bundles
type LocalBundle struct {
	bundle    *types.UDSBundle
	tmpDstDir string
	sourceDir string
	outputDir string
}

// NewLocalBundle creates a new local bundle
func NewLocalBundle(opts *LocalBundleOpts) *LocalBundle {
	return &LocalBundle{
		bundle:    opts.Bundle,
		tmpDstDir: opts.TmpDstDir,
		sourceDir: opts.SourceDir,
		outputDir: opts.OutputDir,
	}
}

// create creates the bundle and outputs to a local tarball
func (lo *LocalBundle) create(createOpts types.BundleCreateOptions) error {
	bundle := lo.bundle
	if bundle.Metadata.Architecture == "" {
		return fmt.Errorf("architecture is required for bundling")
	}
	store, err := ocistore.NewWithContext(context.TODO(), lo.tmpDstDir)
	ctx := context.TODO()

	message.HeaderInfof("🐕 Fetching Packages")

	// create root manifest for bundle, will populate with ref to uds-bundle.yaml and zarf image manifests
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
		pkgDescs, err := pkgFetcher.Fetch()
		if err != nil {
			return err
		}

		// add to artifactPathMap for local bundle tarball
		for _, layer := range pkgDescs {
			digest := layer.Digest.Encoded()
			artifactPathMap[filepath.Join(lo.tmpDstDir, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)
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

	// sign bundle.yaml layer
	if createOpts.SigningKeyPath != "" {
		rootManifest, artifactPathMap, err = lo.signBundle(createOpts, digest, store, artifactPathMap, rootManifest)
		if err != nil {
			return err
		}
	}

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
	rootManifestDesc, err := boci.ToOCIStore(rootManifest, ocispec.MediaTypeImageManifest, store)
	if err != nil {
		return err
	}
	digest = rootManifestDesc.Digest.Encoded()
	artifactPathMap[filepath.Join(lo.tmpDstDir, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)

	// grab index.json
	artifactPathMap[filepath.Join(lo.tmpDstDir, "index.json")] = "index.json"

	// grab oci-layout
	artifactPathMap[filepath.Join(lo.tmpDstDir, "oci-layout")] = "oci-layout"

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

	if lo.outputDir == "" {
		lo.outputDir = lo.sourceDir
	}
	// tarball the bundle
	err = writeTarball(bundle, artifactPathMap, lo.outputDir)
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

	jsonValue, err := utils.JSONValue(bundleYamlDesc)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	message.Debug("Pushed", config.BundleYAML+":", jsonValue)
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
	manifestConfigDesc, err := boci.ToOCIStore(manifestConfig, zoci.ZarfLayerMediaTypeBlob, store)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	return manifestConfigDesc, err
}

// writeTarball builds and writes a bundle tarball to disk based on a file map
func writeTarball(bundle *types.UDSBundle, artifactPathMap types.PathMap, outputDir string) error {
	format := archiver.CompressedArchive{
		Compression: archiver.Zstd{},
		Archival:    archiver.Tar{},
	}
	filename := fmt.Sprintf("%s%s-%s-%s.tar.zst", config.BundlePrefix, bundle.Metadata.Name, bundle.Metadata.Architecture, bundle.Metadata.Version)

	if !helpers.IsDir(outputDir) {
		err := os.MkdirAll(outputDir, 0755)
		if err != nil {
			return err
		}
	}

	dst := filepath.Join(outputDir, filename)

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

	defer archiveBar.Close()

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

// signBundle signs the bundle layer
func (lo *LocalBundle) signBundle(createOpts types.BundleCreateOptions, digest string, store *ocistore.Store, artifactPathMap types.PathMap, rootManifest ocispec.Manifest) (ocispec.Manifest, types.PathMap, error) {
	getSigCreatePassword := func(_ bool) ([]byte, error) {
		if createOpts.SigningKeyPassword != "" {
			return []byte(createOpts.SigningKeyPassword), nil
		}
		if config.CommonOptions.Confirm {
			return nil, nil
		}
		return interactive.PromptSigPassword()
	}
	// sign the bundle layer
	signaturePath := filepath.Join(lo.tmpDstDir, config.BundleYAMLSignature)
	_, err := zarfUtils.CosignSignBlob(filepath.Join(lo.tmpDstDir, config.BlobsDir, digest), signaturePath, createOpts.SigningKeyPath, getSigCreatePassword)
	if err != nil {
		return ocispec.Manifest{}, nil, err
	}

	// append uds-bundle.yaml.sig layer to rootManifest and grab path for archiving
	signatureDesc, err := pushBundleSignature(store, lo.tmpDstDir)
	if err != nil {
		return ocispec.Manifest{}, nil, err
	}
	rootManifest.Layers = append(rootManifest.Layers, signatureDesc)
	digest = signatureDesc.Digest.Encoded()
	artifactPathMap[filepath.Join(lo.tmpDstDir, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)

	jsonValue, err := utils.JSONValue(signatureDesc)
	if err != nil {
		return ocispec.Manifest{}, nil, err
	}
	message.Debug("Pushed", config.BundleYAMLSignature+":", jsonValue)
	return rootManifest, artifactPathMap, nil
}

func pushBundleSignature(store *ocistore.Store, tmpDstDir string) (ocispec.Descriptor, error) {
	ctx := context.TODO()
	signatureBytes, err := os.ReadFile(filepath.Join(tmpDstDir, config.BundleYAMLSignature))
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	signatureDesc := content.NewDescriptorFromBytes(zoci.ZarfLayerMediaTypeBlob, signatureBytes)
	err = store.Push(ctx, signatureDesc, bytes.NewReader(signatureBytes))
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
