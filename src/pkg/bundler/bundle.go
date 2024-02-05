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

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler/fetcher"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler/pusher"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	goyaml "github.com/goccy/go-yaml"
	"github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
)

// createLocalBundle creates the bundle and outputs to a local tarball
func (b *Bundler) createLocalBundle(signature []byte) error {
	bundle := b.bundle
	if bundle.Metadata.Architecture == "" {
		return fmt.Errorf("architecture is required for bundling")
	}
	store, err := ocistore.NewWithContext(context.TODO(), b.tmpDstDir)
	ctx := context.TODO()

	message.HeaderInfof("🐕 Fetching Packages")

	// create root manifest for bundle, will populate with refs to uds-bundle.yaml and zarf image manifests
	rootManifest := ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
	}

	fetcherConfig := fetcher.Config{
		Bundle:             bundle,
		Store:              store,
		TmpDstDir:          b.tmpDstDir,
		NumPkgs:            len(b.bundle.Packages),
		BundleRootManifest: &rootManifest,
	}

	message.Debug("Bundling", bundle.Metadata.Name, "to", b.tmpDstDir)
	if err != nil {
		return err
	}

	artifactPathMap := make(types.PathMap)

	// grab all Zarf pkgs from OCI and put blobs in OCI store
	for i, pkg := range bundle.Packages {
		fetcherConfig.PkgIter = i
		pkgFetcher, err := fetcher.NewFetcher(pkg, fetcherConfig)
		if err != nil {
			return err
		}
		layerDescs, err := pkgFetcher.Fetch()
		if err != nil {
			return err
		}
		// add to artifactPathMap for local tarball
		// todo: if we know the path to where the blobs are stored, we can use that instead of the artifactPathMap?
		for _, layer := range layerDescs {
			digest := layer.Digest.Encoded()
			artifactPathMap[filepath.Join(b.tmpDstDir, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)
		}
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
	artifactPathMap[filepath.Join(b.tmpDstDir, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)

	// create and push bundle manifest config
	manifestConfigDesc, err := pushManifestConfig(store, bundle.Metadata, bundle.Build)
	if err != nil {
		return err
	}
	manifestConfigDigest := manifestConfigDesc.Digest.Encoded()
	artifactPathMap[filepath.Join(b.tmpDstDir, config.BlobsDir, manifestConfigDigest)] = filepath.Join(config.BlobsDir, manifestConfigDigest)

	rootManifest.Config = manifestConfigDesc
	rootManifest.SchemaVersion = 2
	rootManifest.Annotations = manifestAnnotationsFromMetadata(&bundle.Metadata) // maps to registry UI
	rootManifestDesc, err := utils.ToOCIStore(rootManifest, ocispec.MediaTypeImageManifest, store)
	if err != nil {
		return err
	}
	digest = rootManifestDesc.Digest.Encoded()
	artifactPathMap[filepath.Join(b.tmpDstDir, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)

	// grab index.json
	artifactPathMap[filepath.Join(b.tmpDstDir, "index.json")] = "index.json"

	// grab oci-layout
	artifactPathMap[filepath.Join(b.tmpDstDir, "oci-layout")] = "oci-layout"

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
	// todo: no need to tag the local artifact
	err = store.Tag(ctx, rootManifestDesc, bundle.Metadata.Version)
	if err != nil {
		return err
	}
	// ensure the bundle root manifest is the only manifest in the index.json
	err = cleanIndexJSON(b.tmpDstDir, rootManifestDesc)
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

// createRemoteBundle creates the bundle in a remote OCI registry publishes w/ optional signature to the remote repository.
func (b *Bundler) createRemoteBundle(remoteDst *oci.OrasRemote, signature []byte) error {
	bundle := b.bundle
	if bundle.Metadata.Architecture == "" {
		return fmt.Errorf("architecture is required for bundling")
	}
	dstRef := remoteDst.Repo().Reference
	message.Debug("Bundling", bundle.Metadata.Name, "to", dstRef)

	rootManifest := ocispec.Manifest{}
	platform := ocispec.Platform{
		Architecture: config.GetArch(),
		OS:           oci.MultiOS,
	}

	pusherConfig := pusher.Config{
		Bundle:    bundle,
		RemoteDst: remoteDst,
		NumPkgs:   len(bundle.Packages),
	}

	for i, pkg := range bundle.Packages {
		// todo: can leave this block here or move to pusher.NewPusher (would be closer to NewFetcher pattern)
		pkgUrl := fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
		src, err := oci.NewOrasRemote(pkgUrl, platform)
		if err != nil {
			return err
		}
		pusherConfig.RemoteSrc = src
		pkgRootManifest, err := src.FetchRoot()
		if err != nil {
			return err
		}
		pusherConfig.PkgRootManifest = pkgRootManifest
		pusherConfig.PkgIter = i

		remotePusher := pusher.NewPusher(pkg, pusherConfig)
		zarfManifestDesc, err := remotePusher.Push()
		if err != nil {
			return err
		}
		rootManifest.Layers = append(rootManifest.Layers, zarfManifestDesc)
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

	// check for existing index
	index, err := utils.GetIndex(remoteDst, dstRef.String())
	if err != nil {
		return err
	}

	// push bundle root manifest
	rootManifest.Config = configDesc
	rootManifest.SchemaVersion = 2
	rootManifest.Annotations = manifestAnnotationsFromMetadata(&bundle.Metadata) // maps to registry UI
	rootManifestDesc, err := utils.ToOCIRemote(rootManifest, ocispec.MediaTypeImageManifest, remoteDst)
	if err != nil {
		return err
	}

	// create or update, then push index.json
	err = utils.UpdateIndex(index, remoteDst, bundle, rootManifestDesc)
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

// duplicated in bundle.go!
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
func writeTarball(bundle *types.UDSBundle, artifactPathMap types.PathMap) error {
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
