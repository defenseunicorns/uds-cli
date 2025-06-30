// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/mholt/archives"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
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
func (f *localFetcher) GetPkgMetadata() (v1alpha1.ZarfPackage, error) {
	// todo: can we refactor to use Zarf fns?
	tmpDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck

	zarfTarball, err := os.Open(f.cfg.Bundle.Packages[f.cfg.PkgIter].Path)
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}
	if err := config.BundleArchiveFormat.Extract(context.TODO(), zarfTarball, func(_ context.Context, fileInArchive archives.FileInfo) error {
		if fileInArchive.NameInArchive != config.ZarfYAML {
			return nil
		}
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
		return v1alpha1.ZarfPackage{}, err
	}
	zarfYAML := v1alpha1.ZarfPackage{}
	zarfYAMLPath := filepath.Join(tmpDir, config.ZarfYAML)
	err = utils.ReadYAMLStrict(zarfYAMLPath, &zarfYAML)
	if err != nil {
		return v1alpha1.ZarfPackage{}, err
	}
	return zarfYAML, err
}

// toBundle transfers a Zarf package to a given Bundle
func (f *localFetcher) toBundle(pkgTmp string) ([]ocispec.Descriptor, error) {
	ctx := context.TODO()

	loadOpts := packager.LoadOptions{}

	pkgLayout, err := packager.LoadPackage(ctx, f.pkg.Path, loadOpts)

	// get paths from pkgs to put in the bundle
	var pathsToBundle []string

	files, err := pkgLayout.Files()
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		pathsToBundle = append(pathsToBundle, file)
	}

	if len(f.pkg.OptionalComponents) > 0 {
		// check if the images/index.json file exists in the pkgLayout using pkgLayout.GetImagesDirectory()
		imageDir := pkgLayout.GetImageDirPath()
		// check if the index.json file exists in the imageDir
		var imgIndex ocispec.Index
		if _, err := os.Stat(filepath.Join(imageDir, "index.json")); err == nil {
			indexBytes, err := os.ReadFile(filepath.Join(imageDir, "index.json"))
			if err != nil {
				return nil, err
			}
			err = json.Unmarshal(indexBytes, &imgIndex)
			if err != nil {
				return nil, err
			}
		}
		// go into the pkg's image index and filter out optional components, grabbing img manifests of imgs to include
		imgManifestsToInclude, err := boci.FilterImageIndex(pkgLayout.Pkg.Components, imgIndex)
		if err != nil {
			return nil, err
		}

		// go through image index and get all images' config + layers
		includeLayers, err := getImgLayerDigests(imgManifestsToInclude)
		if err != nil {
			return nil, err
		}

		// filter paths to only include layers that are in includeLayers
		filteredPaths := filterPkgPaths(pkgLayout, includeLayers, pkgLayout.Pkg.Components)
		pathsToBundle = filteredPaths
	}

	// create a file store in the same tmp dir as the Zarf pkg (used to create descs + layers)
	src, err := file.New(pkgTmp)
	if err != nil {
		return nil, err
	}

	// go through the paths that should be bundled and add them to the bundle store
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

		// use the file store to create descs + layers that will be used to create the pkg root manifest
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

	// create a pkg root manifest + config because it doesn't come with local Zarf pkgs
	manifestConfigDesc, err := generatePkgManifestConfig(f.cfg.Store, &pkgLayout.Pkg.Metadata, &pkgLayout.Pkg.Build)
	if err != nil {
		return nil, err
	}
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

func generatePkgManifestConfig(store *ocistore.Store, metadata *v1alpha1.ZarfMetadata, build *v1alpha1.ZarfBuildData) (ocispec.Descriptor, error) {
	annotations := map[string]string{
		ocispec.AnnotationTitle:       metadata.Name,
		ocispec.AnnotationDescription: metadata.Description,
	}
	manifestConfig := oci.ConfigPartial{
		Architecture: build.Architecture,
		OCIVersion:   "1.0.1",
		Annotations:  annotations,
	}

	manifestConfigDesc, err := boci.ToOCIStore(manifestConfig, zoci.ZarfConfigMediaType, store)
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

	manifestDesc, err := boci.ToOCIStore(manifest, zoci.ZarfLayerMediaTypeBlob, store)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return manifestDesc, nil
}
