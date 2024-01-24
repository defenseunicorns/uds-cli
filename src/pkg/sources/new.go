// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package sources contains Zarf packager sources
package sources

import (
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	zarfSources "github.com/defenseunicorns/zarf/src/pkg/packager/sources"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
)

// New creates a new package source based on pkgLocation
func New(pkgLocation string, pkgName string, opts zarfTypes.ZarfPackageOptions, sha string) (zarfSources.PackageSource, error) {
	var source zarfSources.PackageSource
	if strings.Contains(pkgLocation, "tar.zst") {
		source = &TarballBundle{
			PkgName:        pkgName,
			PkgOpts:        &opts,
			PkgManifestSHA: sha,
			TmpDir:         opts.PackageSource,
			BundleLocation: pkgLocation,
		}
	} else {
		remote, err := oci.NewOrasRemote(pkgLocation, oci.WithArch(config.GetArch()))
		if err != nil {
			return nil, err
		}
		source = &RemoteBundle{
			PkgName:        pkgName,
			PkgOpts:        &opts,
			PkgManifestSHA: sha,
			TmpDir:         opts.PackageSource,
			Remote:         remote,
		}
	}
	return source, nil
}
