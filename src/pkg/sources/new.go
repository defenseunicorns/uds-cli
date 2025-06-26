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
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
)

// PackageSource is an interface for package sources lifted from Zarf v0.55.6.
//
// While this interface defines three functions, LoadPackage, LoadPackageMetadata, and Collect; only one of them should be used within a packager function.
//
// These functions currently do not promise repeatability due to the side effect nature of loading a package.
//
// Signature and integrity validation is up to the implementation of the package source.
//
//	`sources.ValidatePackageSignature` and `sources.ValidatePackageIntegrity` can be leveraged for this purpose.
type PackageSource interface {
	// NewLoadOptionsForRemove creates a new load options for remove.
	NewLoadOptionsForRemove() packager.LoadOptions

	/* TODO(remove): For Reference Only
	// LoadPackage loads a package from a source.
	LoadPackage(ctx context.Context, dst *layout.PackagePaths, filter filters.ComponentFilterStrategy, unarchiveAll bool) (pkg v1alpha1.ZarfPackage, warnings []string, err error)

	// LoadPackageMetadata loads a package's metadata from a source.
	LoadPackageMetadata(ctx context.Context, dst *layout.PackagePaths, wantSBOM bool, skipValidation bool) (pkg v1alpha1.ZarfPackage, warnings []string, err error)

	// Collect relocates a package from its source to a tarball in a given destination directory.
	Collect(ctx context.Context, destinationDirectory string) (tarball string, err error)
	*/
}

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
		remote, err := zoci.NewRemote(context.TODO(), pkgLocation, platform)
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
