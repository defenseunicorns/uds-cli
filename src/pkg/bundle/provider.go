// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Provider is an interface for processing bundles
//
// operations that are common no matter the source should be implemented on bundler
type Provider interface {
	// LoadBundleMetadata loads a bundle's metadata and signature into the temporary directory and returns a map of the bundle's metadata files
	//
	// these two files are placed in the `dst` directory
	//
	// : if tarball
	// : : extracts the metadata from the tarball
	//
	// : if OCI ref
	// : : pulls the metadata from the OCI ref
	LoadBundleMetadata() (types.PathMap, error)

	// LoadBundle loads a bundle into the temporary directory and returns a map of the bundle's files
	//
	// (currently only the remote provider utilizes the concurrency parameter)
	LoadBundle(options types.BundlePullOptions, concurrency int) (*types.UDSBundle, types.PathMap, error)

	// CreateBundleSBOM creates a bundle-level SBOM from the underlying Zarf packages, if the Zarf package contains an SBOM
	CreateBundleSBOM(extractSBOM bool) error

	// PublishBundle publishes a bundle to a remote OCI repo
	PublishBundle(bundle types.UDSBundle, remote *oci.OrasRemote) error

	// getBundleManifest gets the bundle's root manifest
	getBundleManifest() (*oci.Manifest, error)
}

// NewBundleProvider returns a new bundler Provider based on the source type
func NewBundleProvider(source, destination string) (Provider, error) {
	ctx := context.TODO()
	if helpers.IsOCIURL(source) {
		op := ociProvider{src: source, dst: destination}
		platform := ocispec.Platform{
			Architecture: config.GetArch(),
			OS:           oci.MultiOS,
		}
		// get remote client
		remote, err := zoci.NewRemote(source, platform)
		if err != nil {
			return nil, err
		}
		op.OrasRemote = remote.OrasRemote

		// get root manifest
		root, err := op.FetchRoot(ctx)
		if err != nil {
			return nil, err
		}
		op.rootManifest = root

		return &op, nil
	}
	if !utils.IsValidTarballPath(source) {
		return nil, fmt.Errorf("invalid tarball path: %s", source)
	}
	tp := tarballBundleProvider{ctx: ctx, src: source, dst: destination}
	err := tp.loadBundleManifest()
	if err != nil {
		return nil, err
	}
	return &tp, nil
}
