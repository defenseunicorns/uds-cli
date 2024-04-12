// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package sources contains Zarf packager sources
package sources

import zarfTypes "github.com/defenseunicorns/zarf/src/types"

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
