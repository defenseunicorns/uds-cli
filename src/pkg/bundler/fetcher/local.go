// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfSources "github.com/defenseunicorns/zarf/src/pkg/packager/sources"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
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

	// load pkg and layout of pkg paths
	pkgSrc := zarfSources.TarballSource{
		ZarfPackageOptions: &zarfTypes.ZarfPackageOptions{
			PackageSource: f.pkg.Path,
		},
	}
	pkg, pkgPaths, err := loadPkg(pkgTmp, &pkgSrc, f.pkg.OptionalComponents)
	if err != nil {
		return nil, err
	}

	// get paths from pkgs to put in the bundle
	var pathsToBundle []string
	for _, fullPath := range pkgPaths.Files() {
		pathsToBundle = append(pathsToBundle, fullPath)
	}

	if len(f.pkg.OptionalComponents) > 0 {
		// go into the pkg's image index and filter out optional components, grabbing img manifests of imgs to include
		imgManifestsToInclude, err := filterImageIndex(pkg, pkgPaths.Images.Index)
		if err != nil {
			return nil, err
		}

		// go through image index and get all images' config + layers
		includeLayers, err := getImgLayerDigests(imgManifestsToInclude, pkgPaths)
		if err != nil {
			return nil, err
		}

		// filter paths to only include layers that are in includeLayers, and grab image blobs to recompute checksums
		filteredPaths := filterPkgPaths(pkgPaths, includeLayers)
		pathsToBundle = filteredPaths
	}

	// create a new store to push layers to
	// todo: this is bad....you're writing the Zarf pkg to 3 places
	// 1. pkgTmp 2. this file store 3. bundle store 4. potentiallly the bundle artifact itself....
	src, err := file.New(pkgTmp)
	if err != nil {
		return nil, err
	}

	// go through the filtered paths and add them to the bundle store
	var descs []ocispec.Descriptor
	for _, path := range pathsToBundle {
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
	manifestConfigDesc, err := pushZarfManifestConfigFromMetadata(f.cfg.Store, &pkg.Metadata, &pkg.Build)
	if err != nil {
		return nil, err
	}
	// push the manifest, save the descriptor to put in the bundle root manifest
	rootManifest, err := generatePkgManifest(f.cfg.Store, descs, manifestConfigDesc)
	if err != nil {
		return nil, err
	}
	descs = append(descs, rootManifest, manifestConfigDesc)

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
