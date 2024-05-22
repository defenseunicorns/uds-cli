// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"os"

	"github.com/defenseunicorns/zarf/src/pkg/layout"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
)

// recomputePkgChecksum rewrites the checksums.txt to account for any removed layers
func recomputePkgChecksum(pkgPaths *layout.PackagePaths) (string, error) {
	checksum, err := pkgPaths.GenerateChecksums()
	if err != nil {
		return "", err
	}

	// update zarf.yaml with new aggregate checksum
	var zarfYAML zarfTypes.ZarfPackage
	zarfBytes, err := os.ReadFile(pkgPaths.ZarfYAML)
	if err != nil {
		return "", err
	}
	err = goyaml.Unmarshal(zarfBytes, &zarfYAML)
	if err != nil {
		return "", err
	}
	zarfYAML.Metadata.AggregateChecksum = checksum
	zarfYAMLBytes, err := goyaml.Marshal(zarfYAML)
	if err != nil {
		return "", err
	}
	err = os.WriteFile(pkgPaths.ZarfYAML, zarfYAMLBytes, 0600)
	if err != nil {
		return "", err
	}
	return checksum, nil
}
