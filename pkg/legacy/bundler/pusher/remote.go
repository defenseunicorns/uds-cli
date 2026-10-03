// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package pusher contains functionality to push Zarf pkgs to remote bundles
package pusher

import (
	"context"
	"fmt"
	"io"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/message"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/utils"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/utils/boci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/images"
	zarfoci "github.com/zarf-dev/zarf/src/pkg/oci"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2/content"
)

// RemotePusher contains methods for pulling remote Zarf packages into a bundle
type RemotePusher struct {
	pkg types.Package
	cfg Config
}

// Config contains the configuration for the remote pusher
type Config struct {
	PkgRootManifest *zarfoci.Manifest
	RemoteSrc       zoci.Remote
	RemoteDst       zoci.Remote
	PkgIter         int
	NumPkgs         int
}

// NewPkgPusher creates a pusher object to push Zarf pkgs to a remote bundle
func NewPkgPusher(pkg types.Package, cfg Config) RemotePusher {
	return RemotePusher{pkg: pkg, cfg: cfg}
}

// Push pushes a Zarf pkg to a remote bundle
func (p *RemotePusher) Push() (ocispec.Descriptor, error) {
	zarfManifestDesc, err := boci.ToOCIRemote(p.cfg.PkgRootManifest, layout.ZarfLayerMediaTypeBlob, p.cfg.RemoteDst.OrasRemote)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	url := fmt.Sprintf("%s:%s", p.pkg.Repository, p.pkg.Ref)

	jsonValue, err := utils.JSONValue(*zarfManifestDesc)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	message.Debugf("Pushed %s sub-manifest into %s: %s", url, p.cfg.RemoteDst.Repo().Reference, jsonValue)

	pushSpinner := message.NewProgressSpinner("")
	defer pushSpinner.Stop()

	if err = p.layersToRemoteBundle(pushSpinner); err != nil {
		return ocispec.Descriptor{}, err
	}

	pushSpinner.Successf("Pushed package: %s", p.pkg.Name)
	return *zarfManifestDesc, nil
}

func (p *RemotePusher) layersToRemoteBundle(spinner *message.Spinner) error {
	spinner.Updatef("Fetching %s package layer metadata (package %d of %d)", p.pkg.Name, p.cfg.PkgIter+1, p.cfg.NumPkgs)
	// get only the layers that are required by the components
	layersToCopy, err := boci.FindPkgLayers(p.cfg.RemoteSrc, p.cfg.PkgRootManifest, p.pkg.OptionalComponents)
	if err != nil {
		return err
	}
	spinner.Stop()
	spinner.Updatef("Pushing package %s layers to registry (package %d of %d)", p.pkg.Name, p.cfg.PkgIter+1, p.cfg.NumPkgs)
	err = p.remoteToRemote(layersToCopy)
	if err != nil {
		return err
	}
	return nil
}

// remoteToRemote copies a remote Zarf pkg to a remote OCI registry
func (p *RemotePusher) remoteToRemote(layersToCopy []ocispec.Descriptor) error {
	ctx := context.TODO()
	srcRef := p.cfg.RemoteSrc.Repo().Reference
	dstRef := p.cfg.RemoteDst.Repo().Reference
	// stream copy if different registry
	if srcRef.Registry != dstRef.Registry {
		message.Debugf("Streaming layers from %s --> %s", srcRef, dstRef)
		layers := append(layersToCopy, p.cfg.PkgRootManifest.Config)
		group, groupCtx := errgroup.WithContext(ctx)
		group.SetLimit(max(1, config.CommonOptions.OCIConcurrency))
		seen := make(map[string]struct{}, len(layers))
		var manifests []ocispec.Descriptor
		manifestByDigest := make(map[string]ocispec.Descriptor)
		copyLayer := func(ctx context.Context, layer ocispec.Descriptor) error {
			exists, err := p.cfg.RemoteDst.Repo().Exists(ctx, layer)
			if err != nil || exists {
				return err
			}
			r, err := p.cfg.RemoteSrc.Repo().Fetch(ctx, layer)
			if err != nil {
				return err
			}
			defer r.Close()
			return p.cfg.RemoteDst.Repo().Push(ctx, layer, r)
		}
		for _, layer := range layers {
			if _, ok := seen[layer.Digest.String()]; ok {
				continue
			}
			seen[layer.Digest.String()] = struct{}{}
			if images.IsIndex(layer.MediaType) || images.IsManifest(layer.MediaType) {
				manifests = append(manifests, layer)
				manifestByDigest[layer.Digest.String()] = layer
				continue
			}
			group.Go(func() error {
				return copyLayer(groupCtx, layer)
			})
		}
		if err := group.Wait(); err != nil {
			return err
		}
		// Registries may reject manifests that reference children not yet present.
		// Walk the selected graph so shared children are also pushed first.
		copied := make(map[string]struct{}, len(manifests))
		var copyManifest func(ocispec.Descriptor) error
		copyManifest = func(manifest ocispec.Descriptor) error {
			if _, ok := copied[manifest.Digest.String()]; ok {
				return nil
			}
			successors, err := content.Successors(ctx, p.cfg.RemoteSrc.Repo(), manifest)
			if err != nil {
				return fmt.Errorf("reading image manifest %s: %w", manifest.Digest, err)
			}
			for _, successor := range successors {
				if child, ok := manifestByDigest[successor.Digest.String()]; ok {
					if err := copyManifest(child); err != nil {
						return err
					}
				} else if images.IsIndex(successor.MediaType) || images.IsManifest(successor.MediaType) {
					return fmt.Errorf("image manifest %s references uncopied manifest %s", manifest.Digest, successor.Digest)
				}
			}
			if err := copyLayer(ctx, manifest); err != nil {
				return err
			}
			copied[manifest.Digest.String()] = struct{}{}
			return nil
		}
		for _, manifest := range manifests {
			if err := copyManifest(manifest); err != nil {
				return err
			}
		}
		return nil
	} else {
		// blob mount if same registry
		message.Debugf("Performing a cross repository blob mount on %s from %s --> %s", dstRef, dstRef.Repository, dstRef.Repository)
		spinner := message.NewProgressSpinner("Mounting layers from %s", srcRef.Repository)
		for _, layer := range layersToCopy {
			if layer.Digest == "" {
				continue
			}
			spinner.Updatef("Mounting %s", layer.Digest.Encoded())
			if err := p.cfg.RemoteDst.Repo().Mount(ctx, layer, srcRef.Repository, func() (io.ReadCloser, error) {
				return p.cfg.RemoteSrc.Repo().Fetch(ctx, layer)
			}); err != nil {
				return err
			}
		}
		spinner.Successf("Mounted %d layers", len(layersToCopy))
	}
	return nil
}
