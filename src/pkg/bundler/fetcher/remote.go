// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/cache"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfSources "github.com/defenseunicorns/zarf/src/pkg/packager/sources"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	ocistore "oras.land/oras-go/v2/content/oci"
)

// remoteFetcher fetches remote Zarf pkgs for local bundles
type remoteFetcher struct {
	pkg             types.Package
	cfg             Config
	pkgRootManifest *oci.Manifest
	remote          *zoci.Remote
	pkgTmp          string
}

// Fetch fetches a Zarf pkg and puts it into a local bundle
func (f *remoteFetcher) Fetch() ([]ocispec.Descriptor, error) {
	fetchSpinner := message.NewProgressSpinner("Fetching package %s", f.pkg.Name)
	defer fetchSpinner.Stop()

	pkgTmp, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	if err != nil {
		return nil, err
	}
	pkgSrc := zarfSources.OCISource{
		Remote: f.remote,
		ZarfPackageOptions: &zarfTypes.ZarfPackageOptions{
			PackageSource: f.pkg.Repository,
		},
	}
	pkg, pkgPaths, err := loadPkg(pkgTmp, &pkgSrc, f.pkg.OptionalComponents)
	if err != nil {
		return nil, err
	}

	// go into the pkg's image index and filter out optional components, then rewrite to disk
	imgIndex, err := filterImageIndex(pkg, pkgPaths.Images.Index)
	if err != nil {
		return nil, err
	}

	// go through image index and get all images' config + layers
	includeLayers, err := getImgLayerDigests(imgIndex, pkgPaths)
	if err != nil {
		return nil, err
	}

	// filter paths to only include layers that are in includeLayers, and grab image blobs to recompute checksums
	filteredPaths, imageBlobs := filterPkgPaths(pkgPaths, includeLayers)
	pkgPaths.Images.Blobs = imageBlobs

	// recompute checksums and rewrite to disk
	checksum, err := recomputePkgChecksum(pkgPaths)
	if err != nil {
		return nil, err
	}
	pkg.Metadata.AggregateChecksum = checksum // update pkg metadata already in memory

	fmt.Println(filteredPaths)

	// todo: ok! You've used the same pattern to pull Zarf pkgs both locally and remotely
	// potentially make this into a singular common fn
	// next: need to write logic to bundle the Zarf pkg that you now have above (use the bundle store)

	// find layers in remote
	fetchSpinner.Updatef("Fetching %s package layer metadata (package %d of %d)", f.pkg.Name, f.cfg.PkgIter+1, f.cfg.NumPkgs)
	//layersToCopy, err := utils.FindPkgLayers(*f.remote, f.pkgRootManifest, f.pkg.OptionalComponents)
	//if err != nil {
	//	return nil, err
	//}
	fetchSpinner.Stop()

	// copy layers to local bundle
	//layerDescs, err := f.getRemotePkgLayers(layersToCopy)
	//if err != nil {
	//	return nil, err
	//}
	fetchSpinner.Updatef("Pushing package %s layers to registry (package %d of %d)", f.pkg.Name, f.cfg.PkgIter+1, f.cfg.NumPkgs)

	// todo: maybe use this for loop to clean up the bundle root manifest and the images index
	// write the bundle root manifest using refs to the Zarf root manifests
	//for _, layerDesc := range layerDescs {
	//	// ensure zarf image manifest media type is Zarf blob
	//	if layerDesc.MediaType == ocispec.MediaTypeImageManifest {
	//		layerDesc.MediaType = zoci.ZarfLayerMediaTypeBlob
	//		f.cfg.BundleRootManifest.Layers = append(f.cfg.BundleRootManifest.Layers, layerDesc)
	//	}
	//if layerDesc.Annotations[ocispec.AnnotationTitle] == "images/index.json" {
	//	// todo: shouldn't have to fetch again, but we don't have another ref atm...
	//	pkg, err := f.remote.FetchZarfYAML(context.TODO())
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	// create filter for optional components and apply to the package
	//	createFilter := filters.Combine(
	//		filters.ForDeploy(strings.Join(f.pkg.OptionalComponents, ","), false),
	//	)
	//	components, err := createFilter.Apply(pkg)
	//	if err != nil {
	//		return nil, err
	//	}
	//	pkg.Components = components
	//
	//	// filter the image index to use only images from required + optional components
	//	imgIndexPath := filepath.Join(f.cfg.TmpDstDir, config.BlobsDir, layerDesc.Digest.Encoded())
	//	_, err = filterImageIndex(pkg, imgIndexPath)
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	// re-compute checksum for the index
	//	imgIndexChecksum, err := helpers.GetSHA256OfFile(imgIndexPath)
	//	if err != nil {
	//		return nil, err
	//	}
	//	fmt.Println("Image index checksum: ", imgIndexChecksum)
	//
	//}
	//}

	fetchSpinner.Successf("Fetched package: %s", f.pkg.Name)
	return nil, nil
	//return layerDescs, nil
}

// getRemotePkgLayers copies a remote Zarf pkg to a local OCI store
func (f *remoteFetcher) getRemotePkgLayers(layersToCopy []ocispec.Descriptor) ([]ocispec.Descriptor, error) {
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
		exists, err := checkLayerExists(ctx, layer, f.cfg.Store, f.cfg.TmpDstDir)
		if err != nil {
			return nil, err
		}
		// if layer not in bundle store, add to layersToPull
		// but don't grab Zarf root manifest because we get it automatically during oras.Copy()
		if !exists && layer.MediaType != ocispec.MediaTypeImageManifest {
			layersToPull = append(layersToPull, layer)
			estimatedBytes += layer.Size
		}
		descsToBundle = append(descsToBundle, layer)
	}
	// pull layers that didn't already exist on disk
	if len(layersToPull) > 0 {
		rootPkgDesc, err := f.copyLayers(layersToPull, estimatedBytes)
		if err != nil {
			return nil, err
		}

		// grab pkg root manifest for archiving
		descsToBundle = append(descsToBundle, rootPkgDesc)

		// cache only the image layers that were just pulled
		err = cachePulledImgLayers(layersToPull, f.cfg.TmpDstDir)
		if err != nil {
			return nil, err
		}
	} else {
		// no layers to pull but need to grab pkg root manifest and config manually bc we didn't use oras.Copy()
		pkgManifestDesc, err := utils.ToOCIStore(f.pkgRootManifest, ocispec.MediaTypeImageManifest, f.cfg.Store)
		if err != nil {
			return nil, err
		}
		manifestConfigDesc, err := utils.ToOCIStore(f.pkgRootManifest.Config, zoci.ZarfConfigMediaType, f.cfg.Store)
		if err != nil {
			return nil, err
		}
		descsToBundle = append(descsToBundle, pkgManifestDesc, manifestConfigDesc)
	}
	return descsToBundle, nil
}

// copyLayers uses ORAS to copy layers from a remote repo to a local OCI store
func (f *remoteFetcher) copyLayers(layersToPull []ocispec.Descriptor, estimatedBytes int64) (ocispec.Descriptor, error) {
	// copy Zarf pkg
	copyOpts := utils.CreateCopyOpts(layersToPull, config.CommonOptions.OCIConcurrency)
	// Create a thread to update a progress bar as we save the package to disk
	doneSaving := make(chan error)

	// Grab tmpDirSize and add it to the estimatedBytes, otherwise the progress bar will be off
	// because as multiple packages are pulled into the tmpDir, RenderProgressBarForLocalDirWrite continues to
	// add their size which results in strange MB ratios
	tmpDirSize, err := helpers.GetDirSize(f.cfg.TmpDstDir)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	go zarfUtils.RenderProgressBarForLocalDirWrite(f.cfg.TmpDstDir, estimatedBytes+tmpDirSize, doneSaving, fmt.Sprintf("Pulling bundle: %s", f.pkg.Name), fmt.Sprintf("Successfully pulled package: %s", f.pkg.Name))
	rootPkgDesc, err := oras.Copy(context.TODO(), f.remote.Repo(), f.remote.Repo().Reference.String(), f.cfg.Store, "", copyOpts)
	doneSaving <- err
	<-doneSaving
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return rootPkgDesc, nil
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

// cachePulledImgLayers caches the image layers that were just pulled
func cachePulledImgLayers(pulledLayers []ocispec.Descriptor, dstDir string) (err error) {
	for _, layer := range pulledLayers {
		if strings.Contains(layer.Annotations[ocispec.AnnotationTitle], config.BlobsDir) {
			err = cache.Add(filepath.Join(dstDir, config.BlobsDir, layer.Digest.Encoded()))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// checkLayerExists checks if a layer already exists in the bundle store or the cache
func checkLayerExists(ctx context.Context, layer ocispec.Descriptor, store *ocistore.Store, dstDir string) (bool, error) {
	if exists, _ := store.Exists(ctx, layer); exists {
		return true, nil
	} else if cache.Exists(layer.Digest.Encoded()) {
		err := cache.Use(layer.Digest.Encoded(), filepath.Join(dstDir, config.BlobsDir))
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
