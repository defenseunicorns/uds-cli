// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package sources contains Zarf packager sources
package sources

import (
	"context"
	"fmt"
	"strings"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
)

// NewFromLocation creates a new package source based on pkgLocation
func NewFromLocation(bundleCfg types.BundleConfig, pkg types.Package, opts zarfTypes.ZarfPackageOptions, sha string, nsOverrides NamespaceOverrideMap) (PackageSource, error) {
	var source PackageSource
	var pkgLocation string
	if bundleCfg.DeployOpts.Source != "" {
		pkgLocation = bundleCfg.DeployOpts.Source
	} else if bundleCfg.RemoveOpts.Source != "" {
		pkgLocation = bundleCfg.RemoveOpts.Source
	} else if bundleCfg.InspectOpts.Source != "" {
		pkgLocation = bundleCfg.InspectOpts.Source
	} else {
		return nil, fmt.Errorf("no source provided for package %s", pkg.Name)
	}

	if strings.Contains(pkgLocation, "tar.zst") {
		source = &TarballBundle{
			Pkg:                     pkg,
			PkgOpts:                 &opts,
			PkgManifestSHA:          sha,
			TmpDir:                  opts.PackageSource,
			BundleLocation:          pkgLocation,
			nsOverrides:             nsOverrides,
			PublicKeyPath:           opts.PublicKeyPath,
			SkipSignatureValidation: opts.SkipSignatureValidation,
		}
	} else {
		platform := ocispec.Platform{
			Architecture: config.GetArch(),
			OS:           oci.MultiOS,
		}
		remote, err := zoci.NewRemote(context.TODO(), pkgLocation, platform)
		if err != nil {
			return nil, err
		}
		source = &RemoteBundle{
			Pkg:                     pkg,
			PkgOpts:                 &opts,
			PkgManifestSHA:          sha,
			TmpDir:                  opts.PackageSource,
			Remote:                  remote.OrasRemote,
			nsOverrides:             nsOverrides,
			bundleCfg:               bundleCfg,
			PublicKeyPath:           opts.PublicKeyPath,
			SkipSignatureValidation: opts.SkipSignatureValidation,
		}
	}
	return source, nil
}
