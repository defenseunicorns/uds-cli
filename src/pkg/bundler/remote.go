// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler defines behavior for bundling packages
package bundler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
)

// RemoteBundler contains methods for pulling remote Zarf packages into a bundle
type RemoteBundler struct {
	ctx             context.Context
	pkg             types.BundleZarfPackage
	PkgRootManifest *oci.ZarfOCIManifest
	RemoteSrc       *oci.OrasRemote
	RemoteDst       *oci.OrasRemote
	localDst        *ocistore.Store
}

// NewRemoteBundler creates a bundler to pull remote Zarf pkgs
func NewRemoteBundler(pkg types.BundleZarfPackage, url string, localDst *ocistore.Store, remoteDst *oci.OrasRemote) (RemoteBundler, error) {
	src, err := oci.NewOrasRemote(url)
	if err != nil {
		return RemoteBundler{}, err
	}
	pkgRootManifest, err := src.FetchRoot()
	if err != nil {
		return RemoteBundler{}, err
	}
	if localDst != nil {
		return RemoteBundler{ctx: context.TODO(), RemoteSrc: src, localDst: localDst, PkgRootManifest: pkgRootManifest, pkg: pkg}, err
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
	err = utils.ReadYaml(zarfYAMLPath, &zarfYAML)
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	return zarfYAML, err
}

// PushManifest pushes the Zarf pkg's manifest to either a local or remote bundle
func (b *RemoteBundler) PushManifest() (ocispec.Descriptor, error) {
	pkgManifestBytes, err := json.Marshal(b.PkgRootManifest)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	var zarfManifestDesc ocispec.Descriptor
	if b.localDst != nil {
		// todo: this should have an image manifest media type, but this breaks publish
		zarfManifestDesc = content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, pkgManifestBytes)
		err = b.localDst.Push(b.ctx, zarfManifestDesc, bytes.NewReader(pkgManifestBytes))
	} else {
		zarfManifestDesc, err = b.RemoteDst.PushLayer(pkgManifestBytes, oci.ZarfLayerMediaTypeBlob)
	}
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return zarfManifestDesc, err
}

// PushLayers pushes a Zarf pkg's layers to either a local or remote bundle
func (b *RemoteBundler) PushLayers(spinner *message.Spinner, currentPackageIter int, totalPackages int) ([]ocispec.Descriptor, error) {
	// get only the layers that are required by the components
	spinner.Updatef("Fetching %s package layer metadata (package %d of %d)", b.pkg.Name, currentPackageIter, totalPackages)
	layersToCopy, err := getZarfLayers(b.RemoteSrc, b.pkg, b.PkgRootManifest)
	if err != nil {
		return nil, err
	}
	if b.localDst != nil {
		layerDescs, err := handleLocalCopy(layersToCopy, b, spinner, currentPackageIter, totalPackages)
		if err != nil {
			return nil, err
		}
		// return layer descriptor so we can copy them into the tarball path map
		return layerDescs, err
	}
	spinner.Updatef("Pushing package %s layers to registry (package %d of %d)", b.pkg.Name, currentPackageIter, totalPackages)
	err = handleRemoteCopy(b, layersToCopy)
	if err != nil {
		return nil, err
	}
	return nil, err
}

// handleRemoteCopy copies a remote Zarf pkg to a remote OCI registry
func handleRemoteCopy(b *RemoteBundler, layersToCopy []ocispec.Descriptor) error {
	// stream copy if different registry
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

// handleLocalCopy copies a remote Zarf pkg to a local OCI store
func handleLocalCopy(layersToCopy []ocispec.Descriptor, b *RemoteBundler, spinner *message.Spinner, currentPackageIter int, totalPackages int) ([]ocispec.Descriptor, error) {
	// pull layers from remote and write to OCI artifact dir
	var layerDescs []ocispec.Descriptor
	for i, layer := range layersToCopy {
		if layer.Digest == "" {
			continue
		}
		// check if layer already exists
		if exists, err := b.localDst.Exists(b.ctx, layer); exists {
			continue
		} else if err != nil {
			return nil, err
		}

		spinner.Updatef("Fetching %s layer %d of %d (package %d of %d)", b.pkg.Name, i+1, len(layersToCopy), currentPackageIter, totalPackages)
		layerBytes, err := b.RemoteSrc.FetchLayer(layer)
		if err != nil {
			return nil, err
		}
		layerDesc := content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, layerBytes)
		if err := b.localDst.Push(b.ctx, layerDesc, bytes.NewReader(layerBytes)); err != nil {
			return nil, err
		}
		layerDescs = append(layerDescs, layerDesc)
	}
	return layerDescs, nil
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
