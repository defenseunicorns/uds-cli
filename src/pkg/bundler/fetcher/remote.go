// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/cache"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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

	// find layers in remote
	fetchSpinner.Updatef("Fetching %s package layer metadata (package %d of %d)", f.pkg.Name, f.cfg.PkgIter+1, f.cfg.NumPkgs)
	layersToCopy, err := boci.FindPkgLayers(*f.remote, f.pkgRootManifest, f.pkg.OptionalComponents)
	if err != nil {
		return nil, err
	}
	fetchSpinner.Stop()

	// copy layers to local bundle
	fetchSpinner.Updatef("Pushing package %s layers to bundle (package %d of %d)", f.pkg.Name, f.cfg.PkgIter+1, f.cfg.NumPkgs)
	pkgDescs, err := f.copyRemotePkgLayers(layersToCopy)
	if err != nil {
		return nil, err
	}

	fetchSpinner.Successf("Fetched package: %s", f.pkg.Name)
	return pkgDescs, nil
}

// copyRemotePkgLayers copies a remote Zarf pkg to a local OCI store
func (f *remoteFetcher) copyRemotePkgLayers(layersToCopy []ocispec.Descriptor) ([]ocispec.Descriptor, error) {
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

		exists, err := cache.CheckLayerExists(ctx, layer, f.cfg.Store, f.cfg.TmpDstDir)
		if err != nil {
			return nil, err
		}
		// if layers don't already exist on disk, add to layersToPull
		// but don't grab Zarf root manifest (id'd by image manifest) because we get it automatically during oras.Copy()
		if !exists && layer.MediaType != ocispec.MediaTypeImageManifest {
			layersToPull = append(layersToPull, layer)
			estimatedBytes += layer.Size
		}
		descsToBundle = append(descsToBundle, layer)
	}
	// pull layers that didn't already exist on disk
	if len(layersToPull) > 0 {
		rootPkgDesc, err := cache.CopyLayers(layersToPull, estimatedBytes, f.cfg.TmpDstDir, f.remote.Repo(), f.cfg.Store, f.pkg.Name)
		if err != nil {
			return nil, err
		}

		// grab pkg root manifest for archiving and save it to bundle root manifest
		descsToBundle = append(descsToBundle, rootPkgDesc)
		rootPkgDesc.MediaType = zoci.ZarfLayerMediaTypeBlob // force media type to Zarf blob
		f.cfg.BundleRootManifest.Layers = append(f.cfg.BundleRootManifest.Layers, rootPkgDesc)

		// cache only the image layers that were just pulled
		err = cache.AddPulledImgLayers(layersToPull, f.cfg.TmpDstDir, false)
		if err != nil {
			return nil, err
		}
	} else {
		// no layers to pull but need to grab pkg root manifest and config manually bc we didn't use oras.Copy()
		pkgManifestDesc, err := boci.ToOCIStore(f.pkgRootManifest, ocispec.MediaTypeImageManifest, f.cfg.Store)
		if err != nil {
			return nil, err
		}

		// save pkg manifest to bundle root manifest
		pkgManifestDesc.MediaType = zoci.ZarfLayerMediaTypeBlob // force media type to Zarf blob
		f.cfg.BundleRootManifest.Layers = append(f.cfg.BundleRootManifest.Layers, pkgManifestDesc)

		manifestConfigDesc, err := boci.ToOCIStore(f.pkgRootManifest.Config, zoci.ZarfConfigMediaType, f.cfg.Store)
		if err != nil {
			return nil, err
		}
		descsToBundle = append(descsToBundle, pkgManifestDesc, manifestConfigDesc)
	}
	return descsToBundle, nil
}

func (f *remoteFetcher) GetPkgMetadata() (zarfTypes.ZarfPackage, error) {
	ctx := context.TODO()
	platform := ocispec.Platform{
		Architecture: config.GetArch(),
		OS:           oci.MultiOS,
	}

	// create OCI remote
	url := fmt.Sprintf("%s:%s", f.pkg.Repository, f.pkg.Ref)
	remote, err := zoci.NewRemote(url, platform)
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}

	// get package metadata
	tmpDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	if err != nil {
		return zarfTypes.ZarfPackage{}, fmt.Errorf("bundler unable to create temp directory: %w", err)
	}
	if _, err := remote.PullPackageMetadata(ctx, tmpDir); err != nil {
		return zarfTypes.ZarfPackage{}, err
	}

	// read metadata
	zarfYAML := zarfTypes.ZarfPackage{}
	zarfYAMLPath := filepath.Join(tmpDir, config.ZarfYAML)
	err = utils.ReadYAMLStrict(zarfYAMLPath, &zarfYAML)
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	return zarfYAML, err
}
