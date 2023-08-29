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
	"strings"
)

type RemotePkg struct {
	ctx             context.Context
	pkg             types.BundleZarfPackage
	PkgRootManifest *oci.ZarfOCIManifest
	RemoteSrc       *oci.OrasRemote
	localStore      *ocistore.Store
}

func NewRemotePkg(pkg types.BundleZarfPackage, url string, dst *ocistore.Store) (RemotePkg, error) {
	src, err := oci.NewOrasRemote(url)
	if err != nil {
		return RemotePkg{}, err
	}
	pkgRootManifest, err := src.FetchRoot()
	if err != nil {
		return RemotePkg{}, err
	}
	return RemotePkg{RemoteSrc: src, localStore: dst, PkgRootManifest: pkgRootManifest, pkg: pkg}, err
}

func (r *RemotePkg) GetMetadata(url string, tmpDir string) (zarfTypes.ZarfPackage, error) {
	remote, err := oci.NewOrasRemote(url)
	r.RemoteSrc = remote
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

func (r *RemotePkg) PushManifest() (ocispec.Descriptor, error) {
	pkgManifestBytes, err := json.Marshal(r.PkgRootManifest)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	zarfYamlDesc := content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, pkgManifestBytes)
	err = r.localStore.Push(r.ctx, zarfYamlDesc, bytes.NewReader(pkgManifestBytes))
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return zarfYamlDesc, err
}

func (r *RemotePkg) PushLayers() ([]string, error) {
	// get only the layers that are required by the components
	spinner := message.NewProgressSpinner("Fetching layers from %s", r.RemoteSrc.Repo().Reference.Repository)
	layersToCopy, err := getZarfLayers(r.RemoteSrc, r.pkg, r.PkgRootManifest)
	if err != nil {
		return nil, err
	}
	var digests []string
	// pull layers from remote and write to OCI artifact dir
	for _, layer := range layersToCopy {
		if layer.Digest == "" {
			continue
		}
		// check if layer already exists
		if exists, err := r.localStore.Exists(r.ctx, layer); !exists {
			continue
		} else if err != nil {
			return nil, err
		}

		digest := strings.Split(layer.Digest.String(), "sha256:")[1]
		digests = append(digests, digest)
		spinner.Updatef("Fetching %s", layer.Digest.Encoded())
		layerBytes, err := r.RemoteSrc.FetchLayer(layer)
		if err != nil {
			return nil, err
		}
		layerDesc := content.NewDescriptorFromBytes(oci.ZarfLayerMediaTypeBlob, layerBytes)
		if err := r.localStore.Push(r.ctx, layerDesc, bytes.NewReader(layerBytes)); err != nil {
			return nil, err
		}
	}
	return digests, err
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
