// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundler defines behavior for bundling packages
package bundler

import (
	"context"
	"errors"
	"fmt"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler/pusher"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	"github.com/defenseunicorns/uds-cli/src/types"
	goyaml "github.com/goccy/go-yaml"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
)

// RemoteBundleOpts are the options for creating a remote bundle
type RemoteBundleOpts struct {
	Bundle    *types.UDSBundle
	TmpDstDir string
	Output    string
}

// RemoteBundle enables create ops with remote bundles
type RemoteBundle struct {
	bundle    *types.UDSBundle
	tmpDstDir string
	output    string
}

// NewRemoteBundle creates a new remote bundle
func NewRemoteBundle(opts *RemoteBundleOpts) *RemoteBundle {
	return &RemoteBundle{
		bundle:    opts.Bundle,
		tmpDstDir: opts.TmpDstDir,
		output:    opts.Output,
	}
}

// create creates the bundle in a remote OCI registry publishes w/ optional signature to the remote repository.
func (r *RemoteBundle) create(ctx context.Context, signature []byte) error {
	// set the bundle remote's reference from metadata
	r.output = boci.EnsureOCIPrefix(r.output)
	ref, err := referenceFromMetadata(r.output, &r.bundle.Metadata)
	if err != nil {
		return err
	}
	platform := ocispec.Platform{
		Architecture: config.GetArch(),
		OS:           oci.MultiOS,
	}

	// create the bundle remote
	bundleRemote, err := zoci.NewRemote(ctx, ref, platform)
	if err != nil {
		return err
	}
	bundle := r.bundle
	if bundle.Metadata.Architecture == "" {
		return errors.New("architecture is required for bundling")
	}
	dstRef := bundleRemote.Repo().Reference
	message.Debug("Bundling", bundle.Metadata.Name, "to", dstRef)

	rootManifest := ocispec.Manifest{}
	pusherConfig := pusher.Config{
		Bundle:    bundle,
		RemoteDst: *bundleRemote,
		NumPkgs:   len(bundle.Packages),
	}

	for i, pkg := range bundle.Packages {
		// todo: can leave this block here or move to pusher.NewPkgPusher (would be closer to NewPkgFetcher pattern)
		pkgURL := fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
		src, err := zoci.NewRemote(ctx, pkgURL, platform)
		if err != nil {
			return err
		}
		pusherConfig.RemoteSrc = *src
		pkgRootManifest, err := src.FetchRoot(ctx)
		if err != nil {
			return err
		}
		pusherConfig.PkgRootManifest = pkgRootManifest
		pusherConfig.PkgIter = i

		remotePusher := pusher.NewPkgPusher(pkg, pusherConfig)
		zarfManifestDesc, err := remotePusher.Push()
		if err != nil {
			return err
		}
		rootManifest.Layers = append(rootManifest.Layers, zarfManifestDesc)
	}

	// push the bundle's metadata
	bundleYamlBytes, err := goyaml.Marshal(bundle)
	if err != nil {
		return err
	}
	bundleYamlDesc, err := bundleRemote.PushLayer(ctx, bundleYamlBytes, zoci.ZarfLayerMediaTypeBlob)
	if err != nil {
		return err
	}
	bundleYamlDesc.Annotations = map[string]string{
		ocispec.AnnotationTitle: config.BundleYAML,
	}

	jsonValue, err := utils.JSONValue(bundleYamlDesc)
	if err != nil {
		return err
	}
	message.Debug("Pushed", config.BundleYAML+":", jsonValue)
	rootManifest.Layers = append(rootManifest.Layers, *bundleYamlDesc)

	// push the bundle's signature
	if len(signature) > 0 {
		bundleYamlSigDesc, err := bundleRemote.PushLayer(ctx, signature, zoci.ZarfLayerMediaTypeBlob)
		if err != nil {
			return err
		}
		bundleYamlSigDesc.Annotations = map[string]string{
			ocispec.AnnotationTitle: config.BundleYAMLSignature,
		}
		rootManifest.Layers = append(rootManifest.Layers, *bundleYamlSigDesc)
		jsonValue, err := utils.JSONValue(bundleYamlSigDesc)
		if err != nil {
			return err
		}
		message.Debug("Pushed", config.BundleYAMLSignature+":", jsonValue)
	}

	// push the bundle manifest config
	configDesc, err := pushManifestConfigFromMetadata(bundleRemote.OrasRemote, &bundle.Metadata, &bundle.Build)
	if err != nil {
		return err
	}

	jsonValue, err = utils.JSONValue(configDesc)
	if err != nil {
		return err
	}
	message.Debug("Pushed config:", jsonValue)

	// check for existing index
	index, err := boci.GetIndex(bundleRemote.OrasRemote, dstRef.String())
	if err != nil {
		return err
	}

	// push bundle root manifest
	rootManifest.Config = configDesc
	rootManifest.SchemaVersion = 2
	rootManifest.Annotations = manifestAnnotationsFromMetadata(&bundle.Metadata) // maps to registry UI
	rootManifestDesc, err := boci.ToOCIRemote(rootManifest, ocispec.MediaTypeImageManifest, bundleRemote.OrasRemote)
	if err != nil {
		return err
	}

	// create or update, then push index.json
	err = boci.UpdateIndex(index, bundleRemote.OrasRemote, bundle, *rootManifestDesc)
	if err != nil {
		return err
	}

	message.HorizontalRule()
	flags := ""
	if config.CommonOptions.Insecure {
		flags = "--insecure"
	}
	message.Title("To inspect/deploy/pull:", "")
	message.Command("inspect oci://%s %s", dstRef, flags)
	message.Command("deploy oci://%s %s", dstRef, flags)
	message.Command("pull oci://%s %s", dstRef, flags)

	return nil
}
