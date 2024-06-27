// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package sources contains Zarf packager sources
package sources

import (
	"github.com/defenseunicorns/zarf/src/pkg/packager/filters"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
)

// addNamespaceOverrides checks if pkg components have charts with namespace overrides and adds them
func addNamespaceOverrides(pkg *zarfTypes.ZarfPackage, nsOverrides NamespaceOverrideMap) {
	if len(nsOverrides) == 0 {
		return
	}
	for i, comp := range pkg.Components {
		if _, exists := nsOverrides[comp.Name]; exists {
			for j, chart := range comp.Charts {
				if _, exists = nsOverrides[comp.Name][chart.Name]; exists {
					pkg.Components[i].Charts[j].Namespace = nsOverrides[comp.Name][comp.Charts[j].Name]
				}
			}
		}
	}
}

// setAsYOLO sets the YOLO flag on a package and strips out all images and repos
func setAsYOLO(pkg *zarfTypes.ZarfPackage) {
	pkg.Metadata.YOLO = true
	// strip out all images and repos
	for idx := range pkg.Components {
		pkg.Components[idx].Images = []string{}
		pkg.Components[idx].Repos = []string{}
	}
}

// handleFilter filters components and checks if a package is a partial package by checking its number of components
func handleFilter(pkg zarfTypes.ZarfPackage, filter filters.ComponentFilterStrategy) ([]zarfTypes.ZarfComponent, bool, error) {
	numComponents := len(pkg.Components)
	filteredComps, err := filter.Apply(pkg)
	if err != nil {
		return nil, false, err
	}
	return filteredComps, numComponents > len(filteredComps), nil
}
