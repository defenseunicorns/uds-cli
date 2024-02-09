// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"
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
	LoadBundle(concurrency int) (types.PathMap, error)

	// CreateBundleSBOM creates a bundle-level SBOM from the underlying Zarf packages, if the Zarf package contains an SBOM
	CreateBundleSBOM(extractSBOM bool) error

	// PublishBundle publishes a bundle to a remote OCI repo
	PublishBundle(bundle types.UDSBundle, remote *oci.OrasRemote) error

	// getBundleManifest gets the bundle's root manifest
	getBundleManifest() error

	// ZarfPackageNameMap returns a map of the zarf package name specified in the uds-bundle.yaml to the actual zarf package name
	ZarfPackageNameMap() (map[string]string, error)
}

// NewBundleProvider returns a new bundler Provider based on the source type
func NewBundleProvider(ctx context.Context, source, destination string) (Provider, error) {
	if helpers.IsOCIURL(source) {
		provider := ociProvider{ctx: ctx, src: source, dst: destination}
		platform := ocispec.Platform{
			Architecture: config.GetArch(),
			OS:           oci.MultiOS,
		}
		remote, err := oci.NewOrasRemote(source, platform)
		if err != nil {
			return nil, err
		}
		provider.OrasRemote = remote
		return &provider, nil
	}
	if !utils.IsValidTarballPath(source) {
		return nil, fmt.Errorf("invalid tarball path: %s", source)
	}
	return &tarballBundleProvider{ctx: ctx, src: source, dst: destination}, nil
}
