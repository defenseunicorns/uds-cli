// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package pusher contains functionality to push Zarf pkgs to remote bundles
package pusher

import (
	"context"
	"fmt"
	"io"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// RemotePusher contains methods for pulling remote Zarf packages into a bundle
type RemotePusher struct {
	ctx context.Context
	pkg types.Package
	cfg Config
}

// Config contains the configuration for the remote pusher
type Config struct {
	PkgRootManifest *oci.ZarfOCIManifest
	RemoteSrc       *oci.OrasRemote
	RemoteDst       *oci.OrasRemote
	PkgIter         int
	NumPkgs         int
	Bundle          *types.UDSBundle
}

// NewPkgPusher creates a pusher object to push Zarf pkgs to a remote bundle
func NewPkgPusher(pkg types.Package, cfg Config) RemotePusher {
	return RemotePusher{ctx: context.TODO(), pkg: pkg, cfg: cfg}
}

// Push pushes a Zarf pkg to a remote bundle
func (p *RemotePusher) Push() (ocispec.Descriptor, error) {
	zarfManifestDesc, err := p.PushManifest()
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	// ensure media type is a Zarf blob and append to bundle root manifest
	zarfManifestDesc.MediaType = oci.ZarfLayerMediaTypeBlob
	url := fmt.Sprintf("%s:%s", p.pkg.Repository, p.pkg.Ref)
	message.Debugf("Pushed %s sub-manifest into %s: %s", url, p.cfg.RemoteDst.Repo().Reference, message.JSONValue(zarfManifestDesc))

	// add package name annotations to zarf manifest
	zarfYamlFile, err := p.cfg.RemoteSrc.FetchZarfYAML()
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	zarfManifestDesc.Annotations = make(map[string]string)
	zarfManifestDesc.Annotations[config.UDSPackageNameAnnotation] = p.pkg.Name
	zarfManifestDesc.Annotations[config.ZarfPackageNameAnnotation] = zarfYamlFile.Metadata.Name

	pushSpinner := message.NewProgressSpinner("")
	defer pushSpinner.Stop()

	_, err = p.LayersToRemoteBundle(pushSpinner, p.cfg.PkgIter+1, len(p.cfg.Bundle.Packages))
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	pushSpinner.Successf("Pushed package: %s", p.pkg.Name)
	return zarfManifestDesc, nil
}

// PushManifest pushes the Zarf pkg's manifest to either a local or remote bundle
func (p *RemotePusher) PushManifest() (ocispec.Descriptor, error) {
	var zarfManifestDesc ocispec.Descriptor
	desc, err := utils.ToOCIRemote(p.cfg.PkgRootManifest, oci.ZarfLayerMediaTypeBlob, p.cfg.RemoteDst)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	zarfManifestDesc = desc
	return zarfManifestDesc, nil
}

// LayersToRemoteBundle pushes the Zarf pkg's layers to a remote bundle
func (p *RemotePusher) LayersToRemoteBundle(spinner *message.Spinner, currentPackageIter int, totalPackages int) ([]ocispec.Descriptor, error) {
	spinner.Updatef("Fetching %s package layer metadata (package %d of %d)", p.pkg.Name, currentPackageIter, totalPackages)
	// get only the layers that are required by the components
	layersToCopy, err := utils.GetZarfLayers(p.cfg.RemoteSrc, p.pkg, p.cfg.PkgRootManifest)
	if err != nil {
		return nil, err
	}
	spinner.Stop()
	spinner.Updatef("Pushing package %s layers to registry (package %d of %d)", p.pkg.Name, currentPackageIter, totalPackages)
	err = p.remoteToRemote(layersToCopy)
	if err != nil {
		return nil, err
	}
	return nil, err
}

// remoteToRemote copies a remote Zarf pkg to a remote OCI registry
func (p *RemotePusher) remoteToRemote(layersToCopy []ocispec.Descriptor) error {
	srcRef := p.cfg.RemoteSrc.Repo().Reference
	dstRef := p.cfg.RemoteDst.Repo().Reference
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
		if err := oci.CopyPackage(p.ctx, p.cfg.RemoteSrc, p.cfg.RemoteDst, filterLayers, config.CommonOptions.OCIConcurrency); err != nil {
			return err
		}
	} else {
		// blob mount if same registry
		message.Debugf("Performing a cross repository blob mount on %s from %s --> %s", dstRef, dstRef.Repository, dstRef.Repository)
		spinner := message.NewProgressSpinner("Mounting layers from %s", srcRef.Repository)
		layersToCopy = append(layersToCopy, p.cfg.PkgRootManifest.Config)
		for _, layer := range layersToCopy {
			if layer.Digest == "" {
				continue
			}
			spinner.Updatef("Mounting %s", layer.Digest.Encoded())
			if err := p.cfg.RemoteDst.Repo().Mount(p.ctx, layer, srcRef.Repository, func() (io.ReadCloser, error) {
				return p.cfg.RemoteSrc.Repo().Fetch(p.ctx, layer)
			}); err != nil {
				return err
			}
		}
		spinner.Successf("Mounted %d layers", len(layersToCopy))
	}
	return nil
}
