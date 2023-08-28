// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package zarf defines behavior for interacting with Zarf packages
package zarf

import (
	"context"
	"fmt"
	"github.com/defenseunicorns/uds-cli/src/types"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/oci"
)

// PackageProvider provides methods for interacting with Zarf packages
type PackageProvider interface {
	GetMetadata(path string, tmpDir string) (zarfTypes.ZarfPackage, error)

	// Extract extracts a compressed Zarf archive into a directory
	Extract() error

	// Load loads a zarf.yaml into a Zarf object
	Load() (zarfTypes.ZarfPackage, error)

	// ToBundle transfers a Zarf package to a given Bundle
	ToBundle(store *oci.Store, zarfPkg zarfTypes.ZarfPackage, pathMap map[string]string, bundleTmpDir string, packageTmpDir string) (ocispec.Descriptor, error)
}

// NewPackageProvider creates a new package provider for Zarf pkg operations
func NewPackageProvider(pkg types.BundleZarfPackage, tmpDir string) (PackageProvider, error) {
	if pkg.Repository != "" && pkg.Path != "" {
		return nil, fmt.Errorf("zarf pkg %s cannot have both a repository and a path", pkg.Name)
	}
	if pkg.Repository != "" {
		return nil, nil // todo: implement remote
	} else if pkg.Path != "" {
		return &tarballPkgProvider{tarballSrc: pkg.Path, extractedDst: tmpDir, ctx: context.TODO()}, nil
	}
	return nil, fmt.Errorf("zarf pkg %s must have either a repository or path field", pkg.Name)
}
