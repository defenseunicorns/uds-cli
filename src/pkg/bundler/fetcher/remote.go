// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/cache"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
)

// remoteFetcher fetches remote Zarf pkgs for local bundles
type remoteFetcher struct {
	pkg             types.Package
	cfg             Config
	pkgRootManifest *oci.Manifest
	remote          *zoci.Remote
}

// Fetch fetches a Zarf pkg and puts it into a local bundle
func (f *remoteFetcher) Fetch() ([]ocispec.Descriptor, error) {
	fetchSpinner := message.NewProgressSpinner("Fetching package %s", f.pkg.Name)
	defer fetchSpinner.Stop()

	layerDescs, err := f.layersToLocalBundle(fetchSpinner, f.cfg.PkgIter+1, f.cfg.NumPkgs)
	if err != nil {
		return nil, err
	}

	// grab layers for archiving
	for _, layerDesc := range layerDescs {
		if layerDesc.MediaType == ocispec.MediaTypeImageManifest {
			// rewrite the Zarf image manifest to have media type of Zarf blob
			err = os.Remove(filepath.Join(f.cfg.TmpDstDir, config.BlobsDir, layerDesc.Digest.Encoded()))
			if err != nil {
				return nil, err
			}
			err = utils.FetchLayerAndStore(layerDesc, f.remote.OrasRemote, f.cfg.Store)
			if err != nil {
				return nil, err
			}

			// ensure media type is Zarf blob for layers in the bundle's root manifest
			layerDesc.MediaType = zoci.ZarfLayerMediaTypeBlob

			// add layer to bundle's root manifest
			f.cfg.BundleRootManifest.Layers = append(f.cfg.BundleRootManifest.Layers, layerDesc)
		} else if layerDesc.MediaType == zoci.ZarfConfigMediaType {
			// read in and unmarshal zarf config
			jsonData, err := os.ReadFile(filepath.Join(f.cfg.TmpDstDir, config.BlobsDir, layerDesc.Digest.Encoded()))
			if err != nil {
				return nil, err
			}
			var zarfConfigData oci.ConfigPartial
			err = json.Unmarshal(jsonData, &zarfConfigData)
			if err != nil {
				return nil, err
			}
		}
	}

	fetchSpinner.Successf("Fetched package: %s", f.pkg.Name)
	return layerDescs, nil
}

// LayersToLocalBundle pushes a remote Zarf pkg's layers to a local bundle
func (f *remoteFetcher) layersToLocalBundle(spinner *message.Spinner, currentPackageIter int, totalPackages int) ([]ocispec.Descriptor, error) {
	spinner.Updatef("Fetching %s package layer metadata (package %d of %d)", f.pkg.Name, currentPackageIter, totalPackages)
	// get only the layers that are required by the components
	layersToCopy, err := utils.GetZarfLayers(*f.remote, f.pkgRootManifest, f.pkg.OptionalComponents)
	if err != nil {
		return nil, err
	}
	spinner.Stop()
	layerDescs, err := f.remoteToLocal(layersToCopy)
	if err != nil {
		return nil, err
	}
	// return layer descriptor so we can copy them into the tarball path map
	spinner.Updatef("Pushing package %s layers to registry (package %d of %d)", f.pkg.Name, currentPackageIter, totalPackages)
	return layerDescs, err
}

// remoteToLocal copies a remote Zarf pkg to a local OCI store
func (f *remoteFetcher) remoteToLocal(layersToCopy []ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	ctx := context.TODO()
	// pull layers from remote and write to OCI artifact dir
	var descsToBundle []ocispec.Descriptor
	var layersToPull []ocispec.Descriptor
	estimatedBytes := int64(0)
	// grab descriptors of layers to copy
	for _, layer := range layersToCopy {
		if layer.Digest == "" {
			continue
		}
		// check if layer already exists
		if exists, _ := f.cfg.Store.Exists(ctx, layer); exists {
			continue
		} else if cache.Exists(layer.Digest.Encoded()) {
			err := cache.Use(layer.Digest.Encoded(), filepath.Join(f.cfg.TmpDstDir, config.BlobsDir))
			if err != nil {
				return nil, err
			}
		} else if layer.MediaType != ocispec.MediaTypeImageManifest {
			// grab layer to pull from OCI; don't grab Zarf root manifest because we get it automatically during oras.Copy()
			layersToPull = append(layersToPull, layer)
			estimatedBytes += layer.Size
		}
		descsToBundle = append(descsToBundle, layer)
	}
	// pull layers that didn't exist on disk
	if len(layersToPull) > 0 {
		// copy Zarf pkg
		copyOpts := utils.CreateCopyOpts(layersToPull, config.CommonOptions.OCIConcurrency)
		// Create a thread to update a progress bar as we save the package to disk
		doneSaving := make(chan error)

		// Grab tmpDirSize and add it to the estimatedBytes, otherwise the progress bar will be off
		// because as multiple packages are pulled into the tmpDir, RenderProgressBarForLocalDirWrite continues to
		// add their size which results in strange MB ratios
		tmpDirSize, err := helpers.GetDirSize(f.cfg.TmpDstDir)
		if err != nil {
			return nil, err
		}

		go zarfUtils.RenderProgressBarForLocalDirWrite(f.cfg.TmpDstDir, estimatedBytes+tmpDirSize, doneSaving, fmt.Sprintf("Pulling bundle: %s", f.pkg.Name), fmt.Sprintf("Successfully pulled package: %s", f.pkg.Name))
		rootPkgDesc, err := oras.Copy(context.TODO(), f.remote.Repo(), f.remote.Repo().Reference.String(), f.cfg.Store, "", copyOpts)
		doneSaving <- err
		<-doneSaving
		if err != nil {
			return nil, err
		}

		// grab pkg root manifest for archiving
		descsToBundle = append(descsToBundle, rootPkgDesc)

		// cache only the image layers that were just pulled
		for _, layer := range layersToPull {
			if strings.Contains(layer.Annotations[ocispec.AnnotationTitle], config.BlobsDir) {
				err = cache.Add(filepath.Join(f.cfg.TmpDstDir, config.BlobsDir, layer.Digest.Encoded()))
				if err != nil {
					return nil, err
				}
			}
		}
	} else {
		// need to grab pkg root manifest and config manually bc we didn't use oras.Copy()
		pkgManifestDesc, err := utils.ToOCIStore(f.pkgRootManifest, ocispec.MediaTypeImageManifest, f.cfg.Store)
		if err != nil {
			return nil, err
		}
		descsToBundle = append(descsToBundle, pkgManifestDesc)
	}
	return descsToBundle, nil
}

func (f *remoteFetcher) GetPkgMetadata() (zarfTypes.ZarfPackage, error) {
	ctx := context.TODO()
	platform := ocispec.Platform{
		Architecture: config.GetArch(),
		OS:           oci.MultiOS,
	}
	url := fmt.Sprintf("%s:%s", f.pkg.Repository, f.pkg.Ref)
	remote, err := zoci.NewRemote(url, platform)
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	tmpDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	if err != nil {
		return zarfTypes.ZarfPackage{}, fmt.Errorf("bundler unable to create temp directory: %w", err)
	}
	if _, err := remote.PullPackageMetadata(ctx, tmpDir); err != nil {
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
