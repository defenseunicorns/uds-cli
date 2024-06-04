// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package sources contains Zarf packager sources
package sources

import (
	"fmt"
	"strings"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	zarfSources "github.com/defenseunicorns/zarf/src/pkg/packager/sources"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// New creates a new package source based on pkgLocation
func New(bundleCfg types.BundleConfig, pkg types.Package, opts zarfTypes.ZarfPackageOptions, sha string, nsOverrides NamespaceOverrideMap) (zarfSources.PackageSource, error) {
	var source zarfSources.PackageSource
	var pkgLocation string
	if bundleCfg.DeployOpts.Source != "" {
		pkgLocation = bundleCfg.DeployOpts.Source
	} else if bundleCfg.RemoveOpts.Source != "" {
		pkgLocation = bundleCfg.RemoveOpts.Source
	} else {
		return nil, fmt.Errorf("no source provided for package %s", pkg.Name)
	}

	if strings.Contains(pkgLocation, "tar.zst") {
		source = &TarballBundle{
			Pkg:            pkg,
			PkgOpts:        &opts,
			PkgManifestSHA: sha,
			TmpDir:         opts.PackageSource,
			BundleLocation: pkgLocation,
			nsOverrides:    nsOverrides,
		}
	} else {
		platform := ocispec.Platform{
			Architecture: config.GetArch(),
			OS:           oci.MultiOS,
		}
		remote, err := zoci.NewRemote(pkgLocation, platform)
		if err != nil {
			return nil, err
		}
		source = &RemoteBundle{
			Pkg:            pkg,
			PkgOpts:        &opts,
			PkgManifestSHA: sha,
			TmpDir:         opts.PackageSource,
			Remote:         remote.OrasRemote,
			nsOverrides:    nsOverrides,
			bundleCfg:      bundleCfg,
		}
	}
	return source, nil
}
