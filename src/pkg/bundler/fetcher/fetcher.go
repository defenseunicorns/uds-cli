// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	ocistore "oras.land/oras-go/v2/content/oci"
)

// Fetcher is the interface for fetching packages
type Fetcher interface {
	Fetch() ([]ocispec.Descriptor, error)
	GetPkgMetadata() (zarfTypes.ZarfPackage, error)
}

// Config is the configuration for the fetcher
type Config struct {
	Store              *ocistore.Store
	TmpDstDir          string
	PkgIter            int
	NumPkgs            int
	BundleRootManifest *ocispec.Manifest
	Bundle             *types.UDSBundle
}

// NewPkgFetcher creates a fetcher object to pull Zarf pkgs into a local bundle
func NewPkgFetcher(pkg types.Package, fetcherConfig Config) (Fetcher, error) {
	var fetcher Fetcher
	if utils.IsRemotePkg(pkg) {
		platform := ocispec.Platform{
			Architecture: config.GetArch(),
			OS:           oci.MultiOS,
		}
		url := fmt.Sprintf("%s:%s", pkg.Repository, pkg.Ref)
		remote, err := zoci.NewRemote(url, platform)
		if err != nil {
			return nil, err
		}
		ctx := context.TODO()
		pkgRootManifest, err := remote.FetchRoot(ctx)
		if err != nil {
			return nil, err
		}

		fetcher = &remoteFetcher{
			pkg:             pkg,
			cfg:             fetcherConfig,
			pkgRootManifest: pkgRootManifest,
			remote:          remote,
		}
	} else {
		fetcher = &localFetcher{
			pkg: pkg,
			cfg: fetcherConfig,
		}
	}
	return fetcher, nil
}
