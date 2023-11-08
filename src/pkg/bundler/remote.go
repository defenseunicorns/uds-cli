// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler defines behavior for bundling packages
package bundler

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	ocistore "oras.land/oras-go/v2/content/oci"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/cache"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
)

// RemoteBundler contains methods for pulling remote Zarf packages into a bundle
type RemoteBundler struct {
	ctx             context.Context
	pkg             types.BundleZarfPackage
	PkgRootManifest *oci.ZarfOCIManifest
	RemoteSrc       *oci.OrasRemote
	RemoteDst       *oci.OrasRemote
	localDst        *ocistore.Store
	tmpDir          string
}

// NewRemoteBundler creates a bundler to pull remote Zarf pkgs
// todo: document this fn better or break out into multiple constructors
func NewRemoteBundler(pkg types.BundleZarfPackage, url string, localDst *ocistore.Store, remoteDst *oci.OrasRemote, tmpDir string) (RemoteBundler, error) {
	src, err := oci.NewOrasRemote(url)
	if err != nil {
		return RemoteBundler{}, err
	}
	pkgRootManifest, err := src.FetchRoot()
	if err != nil {
		return RemoteBundler{}, err
	}
	if localDst != nil {
		return RemoteBundler{ctx: context.TODO(), RemoteSrc: src, localDst: localDst, PkgRootManifest: pkgRootManifest, pkg: pkg, tmpDir: tmpDir}, err
	}
	return RemoteBundler{ctx: context.TODO(), RemoteSrc: src, RemoteDst: remoteDst, PkgRootManifest: pkgRootManifest, pkg: pkg}, err
}

// GetMetadata grabs metadata from a remote Zarf package's zarf.yaml
func (b *RemoteBundler) GetMetadata(url string, tmpDir string) (zarfTypes.ZarfPackage, error) {
	remote, err := oci.NewOrasRemote(url)
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	b.RemoteSrc = remote

	if _, err := remote.PullPackageMetadata(tmpDir); err != nil {
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

// PushManifest pushes the Zarf pkg's manifest to either a local or remote bundle
func (b *RemoteBundler) PushManifest() (ocispec.Descriptor, error) {
	var zarfManifestDesc ocispec.Descriptor
	if b.localDst != nil {
		desc, err := utils.ToOCIStore(b.PkgRootManifest, oci.ZarfLayerMediaTypeBlob, b.localDst)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
		zarfManifestDesc = desc
	} else {
		desc, err := utils.ToOCIRemote(b.PkgRootManifest, oci.ZarfLayerMediaTypeBlob, b.RemoteDst)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
		zarfManifestDesc = desc
	}
	return zarfManifestDesc, nil
}

// LayersToBundle pushes a remote Zarf pkg's layers to either a local or remote bundle
func (b *RemoteBundler) LayersToBundle(spinner *message.Spinner, currentPackageIter int, totalPackages int) ([]ocispec.Descriptor, error) {
	spinner.Updatef("Fetching %s package layer metadata (package %d of %d)", b.pkg.Name, currentPackageIter, totalPackages)
	// get only the layers that are required by the components
	layersToCopy, err := getZarfLayers(b.RemoteSrc, b.pkg, b.PkgRootManifest)
	if err != nil {
		return nil, err
	}
	spinner.Stop()
	if b.localDst != nil {
		layerDescs, err := b.remoteToLocal(layersToCopy)
		if err != nil {
			return nil, err
		}
		// return layer descriptor so we can copy them into the tarball path map
		return layerDescs, err
	}
	spinner.Updatef("Pushing package %s layers to registry (package %d of %d)", b.pkg.Name, currentPackageIter, totalPackages)
	err = b.remoteToRemote(layersToCopy)
	if err != nil {
		return nil, err
	}
	return nil, err
}

// remoteToRemote copies a remote Zarf pkg to a remote OCI registry
func (b *RemoteBundler) remoteToRemote(layersToCopy []ocispec.Descriptor) error {
	srcRef := b.RemoteSrc.Repo().Reference
	dstRef := b.RemoteDst.Repo().Reference
	// stream copy if different registry
	if srcRef.Registry != dstRef.Registry {
		message.Debugf("Streaming layers from %s --> %s", srcRef, dstRef)

		// filterLayers returns true if the layer is in the list of layers to copy, this allows for
		// copying only the layers that are required by the required + specified optional components
		filterLayers := func(d ocispec.Descriptor) bool {
			for _, layer := range layersToCopy {
				if layer.Digest == d.Digest {
					return true
				}
			}
			return false
		}
		if err := oci.CopyPackage(b.ctx, b.RemoteSrc, b.RemoteDst, filterLayers, config.CommonOptions.OCIConcurrency); err != nil {
			return err
		}
	} else {
		// blob mount if same registry
		message.Debugf("Performing a cross repository blob mount on %s from %s --> %s", dstRef, dstRef.Repository, dstRef.Repository)
		spinner := message.NewProgressSpinner("Mounting layers from %s", srcRef.Repository)
		layersToCopy = append(layersToCopy, b.PkgRootManifest.Config)
		for _, layer := range layersToCopy {
			if layer.Digest == "" {
				continue
			}
			spinner.Updatef("Mounting %s", layer.Digest.Encoded())
			if err := b.RemoteDst.Repo().Mount(b.ctx, layer, srcRef.Repository, func() (io.ReadCloser, error) {
				return b.RemoteSrc.Repo().Fetch(b.ctx, layer)
			}); err != nil {
				return err
			}
		}
		spinner.Successf("Mounted %d layers", len(layersToCopy))
	}
	return nil
}

// remoteToLocal copies a remote Zarf pkg to a local OCI store
func (b *RemoteBundler) remoteToLocal(layersToCopy []ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	// pull layers from remote and write to OCI artifact dir
	var layerDescsToArchive []ocispec.Descriptor
	var layersToPull []ocispec.Descriptor
	estimatedBytes := int64(0)
	// grab descriptors of layers to copy
	for _, layer := range layersToCopy {
		if layer.Digest == "" {
			continue
		}
		// check if layer already exists
		if exists, _ := b.localDst.Exists(b.ctx, layer); exists {
			continue
		} else if cache.Exists(layer.Digest.Encoded()) {
			err := cache.Use(layer.Digest.Encoded(), filepath.Join(b.tmpDir, config.BlobsDir))
			if err != nil {
				return nil, err
			}
			layerDescsToArchive = append(layerDescsToArchive, layer)
			continue
		}
		// grab layer to pull from OCI
		if layer.MediaType != ocispec.MediaTypeImageManifest {
			layersToPull = append(layersToPull, layer)
			layerDescsToArchive = append(layerDescsToArchive, layer)
			estimatedBytes += layer.Size
			continue
		}
		layerDescsToArchive = append(layerDescsToArchive, layer)
	}
	// pull layers that didn't exist on disk
	if len(layersToPull) > 0 {
		// copy bundle
		copyOpts := utils.CreateCopyOpts(layersToPull, config.CommonOptions.OCIConcurrency)
		// Create a thread to update a progress bar as we save the package to disk
		doneSaving := make(chan int)
		var wg sync.WaitGroup
		wg.Add(1)
		go zarfUtils.RenderProgressBarForLocalDirWrite(b.tmpDir, estimatedBytes, &wg, doneSaving, fmt.Sprintf("Pulling bundle: %s", b.pkg.Name), fmt.Sprintf("Successfully pulled bundle: %s", b.pkg.Name))
		rootPkgDesc, err := oras.Copy(context.TODO(), b.RemoteSrc.Repo(), b.RemoteSrc.Repo().Reference.String(), b.localDst, "", copyOpts)
		if err != nil {
			doneSaving <- 1
			return nil, err
		}
		doneSaving <- 1
		wg.Wait()

		// grab pkg root manifest for archiving
		layerDescsToArchive = append(layerDescsToArchive, rootPkgDesc)

		// cache only the image layers that were just pulled
		for _, layer := range layersToPull {
			if strings.Contains(layer.Annotations[ocispec.AnnotationTitle], config.BlobsDir) {
				err = cache.Add(filepath.Join(b.tmpDir, config.BlobsDir, layer.Digest.Encoded()))
				if err != nil {
					return nil, err
				}
			}
		}
	} else {
		// need to grab pkg root manifest manually bc we didn't use oras.Copy()
		pkgManifestDesc, err := utils.ToOCIStore(b.PkgRootManifest, ocispec.MediaTypeImageManifest, b.localDst)
		if err != nil {
			return nil, err
		}
		layerDescsToArchive = append(layerDescsToArchive, pkgManifestDesc)
	}
	return layerDescsToArchive, nil
}

// getZarfLayers grabs the necessary Zarf pkg layers from a remote OCI registry
func getZarfLayers(remote *oci.OrasRemote, pkg types.BundleZarfPackage, pkgRootManifest *oci.ZarfOCIManifest) ([]ocispec.Descriptor, error) {
	layersFromComponents, err := remote.LayersFromRequestedComponents(pkg.OptionalComponents)
	if err != nil {
		return nil, err
	}
	// get the layers that are always pulled
	var metadataLayers []ocispec.Descriptor
	for _, path := range oci.PackageAlwaysPull {
		layer := pkgRootManifest.Locate(path)
		if !oci.IsEmptyDescriptor(layer) {
			metadataLayers = append(metadataLayers, layer)
		}
	}
	layersToCopy := append(layersFromComponents, metadataLayers...)
	layersToCopy = append(layersToCopy, pkgRootManifest.Config)
	return layersToCopy, err
}
