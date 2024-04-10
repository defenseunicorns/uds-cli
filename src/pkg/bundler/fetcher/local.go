// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	av3 "github.com/mholt/archiver/v3"
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

	err = f.extract()
	if err != nil {
		return nil, err
	}

	zarfPkg, err := f.load()
	if err != nil {
		return nil, err
	}

	layerDescs, err := f.toBundle(zarfPkg, pkgTmp)
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
	err = zarfUtils.ReadYaml(zarfYAMLPath, &zarfYAML)
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	return zarfYAML, err
}

// extract extracts a compressed Zarf archive into a directory
func (f *localFetcher) extract() error {
	err := av3.Unarchive(f.pkg.Path, f.extractDst) // todo: awkward to use old version of mholt/archiver
	if err != nil {
		return err
	}
	return nil
}

// load loads a zarf.yaml into a Zarf object
func (f *localFetcher) load() (zarfTypes.ZarfPackage, error) {
	// grab zarf.yaml from extracted archive
	p, err := os.ReadFile(filepath.Join(f.extractDst, config.ZarfYAML))
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	var pkg zarfTypes.ZarfPackage
	if err := goyaml.Unmarshal(p, &pkg); err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	return pkg, err
}

// toBundle transfers a Zarf package to a given Bundle
func (f *localFetcher) toBundle(pkg zarfTypes.ZarfPackage, pkgTmp string) ([]ocispec.Descriptor, error) {
	// todo: only grab components that are required + specified in optionalComponents
	ctx := context.TODO()
	src, err := file.New(pkgTmp)
	if err != nil {
		return nil, err
	}
	// Grab Zarf layers
	var paths []string
	err = filepath.Walk(pkgTmp, func(path string, info os.FileInfo, err error) error {
		// Catch any errors that happened during the walk
		if err != nil {
			return err
		}

		// Add any resource that is not a directory to the paths of objects we will include into the package
		if !info.IsDir() {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("unable to get the layers in the package to publish: %w", err)
	}

	var descs []ocispec.Descriptor
	for _, path := range paths {
		name, err := filepath.Rel(pkgTmp, path)
		if err != nil {
			return nil, err
		}

		mediaType := zoci.ZarfLayerMediaTypeBlob

		// todo: try finding the desc with media type of image manifest, and rewrite it here!
		// just iterate through it's layers and add the annotations to each layer, then push to the store and add to descs

		// adds title annotations to descs and creates layer to put in the store
		// title annotations need to be added to the pkg root manifest
		// Zarf image manifests already contain those title annotations in remote OCI repos, but they need to be added manually here

		// if using a custom tmp dir that is not an absolute path, get working dir and prepend to path to make it absolute
		if !filepath.IsAbs(path) {
			wd, err := os.Getwd()
			if err != nil {
				return nil, err
			}
			path = filepath.Join(wd, path)
		}

		desc, err := src.Add(ctx, name, mediaType, path)
		if err != nil {
			return nil, err
		}
		layer, err := src.Fetch(ctx, desc)
		if err != nil {
			return nil, err
		}

		// push if layer doesn't already exist in bundleStore
		// at this point, for some reason, many layers already exist in the store?
		if exists, err := f.cfg.Store.Exists(ctx, desc); !exists && err == nil {
			if err := f.cfg.Store.Push(ctx, desc, layer); err != nil {
				return nil, err
			} else if err != nil {
				return nil, err
			}
		}
		descs = append(descs, desc)
	}

	// push the manifest config
	// todo: I don't think this is making it to the local bundle
	manifestConfigDesc, err := pushZarfManifestConfigFromMetadata(f.cfg.Store, &pkg.Metadata, &pkg.Build)
	if err != nil {
		return nil, err
	}
	// push the manifest
	rootManifest, err := generatePkgManifest(f.cfg.Store, descs, manifestConfigDesc)
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
