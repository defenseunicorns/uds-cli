// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package bundler

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	ocistore "oras.land/oras-go/v2/content/oci"
	"path/filepath"
)

type RemoteBundler struct {
	ctx             context.Context
	pkg             types.BundleZarfPackage
	PkgRootManifest *oci.ZarfOCIManifest
	RemoteSrc       *oci.OrasRemote
	localDst        *ocistore.Store
	remoteDst       *oci.OrasRemote
}

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
		return RemoteBundler{RemoteSrc: src, localDst: localDst, PkgRootManifest: pkgRootManifest, pkg: pkg}, err
	}
	return RemoteBundler{RemoteSrc: src, remoteDst: remoteDst, PkgRootManifest: pkgRootManifest, pkg: pkg}, err
}

func (b *RemoteBundler) GetMetadata(url string, tmpDir string) (zarfTypes.ZarfPackage, error) {
	remote, err := oci.NewOrasRemote(url)
	b.RemoteSrc = remote
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}

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

func (b *RemoteBundler) PushManifest() (ocispec.Descriptor, error) {
	pkgManifestBytes, err := json.Marshal(b.PkgRootManifest)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	zarfManifestDesc := content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, pkgManifestBytes)
	err = b.localDst.Push(b.ctx, zarfManifestDesc, bytes.NewReader(pkgManifestBytes))
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return zarfManifestDesc, err
}

func (b *RemoteBundler) PushLayers() ([]ocispec.Descriptor, error) {
	// get only the layers that are required by the components
	spinner := message.NewProgressSpinner("Fetching layers from %s", b.RemoteSrc.Repo().Reference.Repository)
	layersToCopy, err := getZarfLayers(b.RemoteSrc, b.pkg, b.PkgRootManifest)
	if err != nil {
		return nil, err
	}
	var layerDescs []ocispec.Descriptor
	// pull layers from remote and write to OCI artifact dir
	for _, layer := range layersToCopy {
		if layer.Digest == "" {
			continue
		}
		// check if layer already exists
		if exists, err := b.localDst.Exists(b.ctx, layer); exists {
			continue
		} else if err != nil {
			return nil, err
		}

		spinner.Updatef("Fetching %s", layer.Digest.Encoded())
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
	return layerDescs, err
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
