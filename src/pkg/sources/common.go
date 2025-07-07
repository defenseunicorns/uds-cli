// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package sources contains Zarf packager sources
package sources

import (
	"context"

	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

type PackageSource interface {
	// LoadPackage loads a package from a source.
	LoadPackage(ctx context.Context, filter filters.ComponentFilterStrategy, unarchiveAll bool) (pkgLayout *layout.PackageLayout, warnings []string, err error)

	// // LoadPackageMetadata loads a package's metadata from a source.
	// LoadPackageMetadata(ctx context.Context, wantSBOM bool, skipValidation bool) (pkg v1alpha1.ZarfPackage, warnings []string, err error)

	// // Collect relocates a package from its source to a tarball in a given destination directory.
	// Collect(ctx context.Context, destinationDirectory string) (tarball string, err error)
}

// addNamespaceOverrides checks if pkg components have charts with namespace overrides and adds them
// func addNamespaceOverrides(pkg *v1alpha1.ZarfPackage, nsOverrides NamespaceOverrideMap) {
// 	if len(nsOverrides) == 0 {
// 		return
// 	}
// 	for i, comp := range pkg.Components {
// 		if _, exists := nsOverrides[comp.Name]; exists {
// 			for j, chart := range comp.Charts {
// 				if _, exists = nsOverrides[comp.Name][chart.Name]; exists {
// 					pkg.Components[i].Charts[j].Namespace = nsOverrides[comp.Name][comp.Charts[j].Name]
// 				}
// 			}
// 		}
// 	}
// }

// setAsYOLO sets the YOLO flag on a package and strips out all images and repos
func setAsYOLO(pkg *v1alpha1.ZarfPackage) {
	pkg.Metadata.YOLO = true
	// strip out all images and repos
	for idx := range pkg.Components {
		pkg.Components[idx].Images = []string{}
		pkg.Components[idx].Repos = []string{}
	}
}

// handleFilter filters components and checks if a package is a partial package by checking its number of components
func handleFilter(pkg v1alpha1.ZarfPackage, filter filters.ComponentFilterStrategy) ([]v1alpha1.ZarfComponent, bool, error) {
	numComponents := len(pkg.Components)
	filteredComps, err := filter.Apply(pkg)
	if err != nil {
		return nil, false, err
	}
	return filteredComps, numComponents > len(filteredComps), nil
}
