// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/layout"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/packager/filters"
	zarfSources "github.com/defenseunicorns/zarf/src/pkg/packager/sources"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	av4 "github.com/mholt/archiver/v4"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/file"
	ocistore "oras.land/oras-go/v2/content/oci"
)

type localFetcher struct {
	pkg        types.Package
	cfg        Config
	extractDst string
}

// Fetch fetches a local Zarf pkg and puts it into a local bundle
func (f *localFetcher) Fetch() ([]ocispec.Descriptor, error) {
	fetchSpinner := message.NewProgressSpinner("Fetching package %s", f.pkg.Name)
	defer fetchSpinner.Stop()
	pkgTmp, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	defer os.RemoveAll(pkgTmp)
	if err != nil {
		return nil, err
	}
	f.extractDst = pkgTmp

	layerDescs, err := f.toBundle(pkgTmp)
	if err != nil {
		return nil, err
	}
	fetchSpinner.Successf("Fetched package: %s", f.pkg.Name)
	return layerDescs, nil
}

// GetPkgMetadata grabs metadata from a local Zarf package's zarf.yaml
func (f *localFetcher) GetPkgMetadata() (zarfTypes.ZarfPackage, error) {
	tmpDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck

	zarfTarball, err := os.Open(f.cfg.Bundle.Packages[f.cfg.PkgIter].Path)
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	format := av4.CompressedArchive{
		Compression: av4.Zstd{},
		Archival:    av4.Tar{},
	}
	if err := format.Extract(context.TODO(), zarfTarball, []string{config.ZarfYAML}, func(_ context.Context, fileInArchive av4.File) error {
		// write zarf.yaml to tmp for checking optional components later on
		dst := filepath.Join(tmpDir, fileInArchive.NameInArchive)
		outFile, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer outFile.Close()
		stream, err := fileInArchive.Open()
		if err != nil {
			return err
		}
		defer stream.Close()
		_, err = io.Copy(outFile, io.Reader(stream))
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		zarfTarball.Close()
		return zarfTypes.ZarfPackage{}, err
	}
	zarfYAML := zarfTypes.ZarfPackage{}
	zarfYAMLPath := filepath.Join(tmpDir, config.ZarfYAML)
	err = utils.ReadYAMLStrict(zarfYAMLPath, &zarfYAML)
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	return zarfYAML, err
}

// toBundle transfers a Zarf package to a given Bundle
func (f *localFetcher) toBundle(pkgTmp string) ([]ocispec.Descriptor, error) {
	ctx := context.TODO()

	// todo: test the case of an optional component that only has an action (maybe also test for charts and manifests)

	// create a new layout for the package to make it easy to filter components and get file paths
	pkgPaths := layout.New(pkgTmp)
	tarballSrc := zarfSources.TarballSource{
		ZarfPackageOptions: &zarfTypes.ZarfPackageOptions{
			PackageSource: f.pkg.Path,
		},
	}

	// filter out optional components
	createFilter := filters.Combine(
		filters.ForDeploy(strings.Join(f.pkg.OptionalComponents, ","), false),
	)

	// calling LoadPackage populates the pkgPaths with the files from the tarball
	pkg, _, err := tarballSrc.LoadPackage(pkgPaths, createFilter, false)
	if err != nil {
		return nil, err
	}

	paths := pkgPaths.Files()

	// don't include any images from non-required components
	// read in images/index.json
	var imgIndex ocispec.Index
	if pkgPaths.Images.Index != "" {
		indexBytes, err := os.ReadFile(pkgPaths.Images.Index)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(indexBytes, &imgIndex)
		if err != nil {
			return nil, err
		}
	}

	// include only images that are in the components using a map to dedup manifests
	manifestIncludeMap := map[string]ocispec.Descriptor{}
	for _, manifest := range imgIndex.Manifests {
		for _, component := range pkg.Components {
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
	imgIndex.Manifests = manifestsToInclude

	// rewrite the images index (desc will be rewritten when its copied to the bundle)
	if len(imgIndex.Manifests) > 0 {
		imgIndexBytes, err := json.Marshal(imgIndex)
		if err != nil {
			return nil, err
		}
		err = os.WriteFile(pkgPaths.Images.Index, imgIndexBytes, 0600)
		if err != nil {
			return nil, err
		}

	}

	// go to image manifest and grab config + layers
	var includeLayers []string
	for _, manifest := range imgIndex.Manifests {
		includeLayers = append(includeLayers, manifest.Digest.Hex()) // be sure to include image manifest
		manifestBytes, err := os.ReadFile(filepath.Join(pkgPaths.Images.Base, config.BlobsDir, manifest.Digest.Hex()))
		if err != nil {
			return nil, err
		}
		var imgManifest ocispec.Manifest
		err = goyaml.Unmarshal(manifestBytes, &imgManifest)
		if err != nil {
			return nil, err
		}
		includeLayers = append(includeLayers, imgManifest.Config.Digest.Hex()) // don't forget the config
		for _, layer := range imgManifest.Layers {
			includeLayers = append(includeLayers, layer.Digest.Hex())
		}
	}

	// filter paths to only include layers that are in includeLayers
	var filteredPaths []string
	var imageBlobs []string
	for _, path := range paths {
		// include all paths that aren't in the blobs dir
		if !strings.Contains(path, config.BlobsDir) {
			filteredPaths = append(filteredPaths, path)
			continue
		}
		// include paths that are in the blobs dir and are in includeLayers
		for _, layer := range includeLayers {
			if strings.Contains(path, config.BlobsDir) && strings.Contains(path, layer) {
				filteredPaths = append(filteredPaths, path)
				imageBlobs = append(imageBlobs, path) // save off image blobs so we can rewrite pkgPaths (makes generating checksums easier)
				break
			}
		}
	}

	// ensure zarf.yaml, checksums and SBOMS (if exists) are always included
	// note you may have extra SBOMs because they are not filtered out
	alwaysInclude := []string{pkgPaths.ZarfYAML, pkgPaths.Checksums}
	if pkgPaths.SBOMs.Path != "" {
		alwaysInclude = append(alwaysInclude, pkgPaths.SBOMs.Path)
	}
	filteredPaths = helpers.MergeSlices(filteredPaths, alwaysInclude, func(a, b string) bool {
		return a == b
	})

	// rewrite checksums.txt with removed layers
	pkgPaths.Images.Blobs = imageBlobs
	checksum, err := recomputePkgChecksum(pkgPaths)
	if err != nil {
		return nil, err
	}
	pkg.Metadata.AggregateChecksum = checksum // update pkg metadata already in memory

	// create a new store to push layers to
	src, err := file.New(pkgTmp)
	if err != nil {
		return nil, err
	}

	// go through the filtered paths and add them to the bundle store
	var descs []ocispec.Descriptor
	for _, path := range filteredPaths {
		name, err := filepath.Rel(pkgTmp, path)
		if err != nil {
			return nil, err
		}

		// set media type to blob for all layers in the pkg
		mediaType := zoci.ZarfLayerMediaTypeBlob

		// if using a custom tmp dir that is not an absolute path, get working dir and prepend to path to make it absolute
		if !filepath.IsAbs(path) {
			wd, err := os.Getwd()
			if err != nil {
				return nil, err
			}
			path = filepath.Join(wd, path)
		}

		// Zarf image manifests already contain those title annotations in remote OCI repos, but they need to be added manually here
		// computer descriptors for each layer (we get title annotations for free)
		desc, err := src.Add(ctx, name, mediaType, path)
		if err != nil {
			return nil, err
		}
		layer, err := src.Fetch(ctx, desc)
		if err != nil {
			return nil, err
		}

		// push if layer to bundle store if it doesn't already exist
		if exists, err := f.cfg.Store.Exists(ctx, desc); !exists && err == nil {
			if err := f.cfg.Store.Push(ctx, desc, layer); err != nil {
				return nil, err
			}
		}

		// record descriptor for the pkg root manifest
		descs = append(descs, desc)
	}

	// push the manifest config
	// todo: I don't think this is making it to the local bundle
	manifestConfigDesc, err := pushZarfManifestConfigFromMetadata(f.cfg.Store, &pkg.Metadata, &pkg.Build)
	if err != nil {
		return nil, err
	}
	// push the manifest, save the descriptor to put in the bundle root manifest
	rootManifest, err := generatePkgManifest(f.cfg.Store, descs, manifestConfigDesc)
	if err != nil {
		return nil, err
	}
	descs = append(descs, rootManifest)

	// put digest in uds-bundle.yaml to reference during deploy
	f.cfg.Bundle.Packages[f.cfg.PkgIter].Ref = f.cfg.Bundle.Packages[f.cfg.PkgIter].Ref + "@" + rootManifest.Digest.String()

	// append zarf image manifest to bundle root manifest and grab path for archiving
	f.cfg.BundleRootManifest.Layers = append(f.cfg.BundleRootManifest.Layers, rootManifest)
	return descs, err
}

func pushZarfManifestConfigFromMetadata(store *ocistore.Store, metadata *zarfTypes.ZarfMetadata, build *zarfTypes.ZarfBuildData) (ocispec.Descriptor, error) {
	annotations := map[string]string{
		ocispec.AnnotationTitle:       metadata.Name,
		ocispec.AnnotationDescription: metadata.Description,
	}
	manifestConfig := oci.ConfigPartial{
		Architecture: build.Architecture,
		OCIVersion:   "1.0.1",
		Annotations:  annotations,
	}

	manifestConfigDesc, err := utils.ToOCIStore(manifestConfig, ocispec.MediaTypeImageManifest, store)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return manifestConfigDesc, err
}

func generatePkgManifest(store *ocistore.Store, descs []ocispec.Descriptor, configDesc ocispec.Descriptor) (ocispec.Descriptor, error) {
	// adopted from oras.Pack fn; manually build the manifest and push to store and save reference
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2, // historical value. does not pertain to OCI or docker version
		},
		Config:    configDesc,
		MediaType: zoci.ZarfLayerMediaTypeBlob,
		Layers:    descs,
	}

	manifestDesc, err := utils.ToOCIStore(manifest, zoci.ZarfLayerMediaTypeBlob, store)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return manifestDesc, nil
}
