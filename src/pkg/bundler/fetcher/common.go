// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	goyaml "github.com/goccy/go-yaml"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/layout"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	zarfSources "github.com/zarf-dev/zarf/src/pkg/packager/sources"
)

// loadPkg loads a package from a tarball source and filters out optional components
func loadPkg(pkgTmp string, pkgSrc zarfSources.PackageSource, optionalComponents []string) (v1alpha1.ZarfPackage, *layout.PackagePaths, error) {
	// create empty layout and source
	pkgPaths := layout.New(pkgTmp)

	// create filter for optional components
	createFilter := filters.Combine(
		filters.ForDeploy(strings.Join(optionalComponents, ","), false),
	)

	// load the package with the filter (calling LoadPackage populates the pkgPaths with the files from the tarball)
	pkg, _, err := pkgSrc.LoadPackage(context.TODO(), pkgPaths, createFilter, false)
	if err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}
	return pkg, pkgPaths, nil
}

// getImgLayerDigests grabs the digests of the layers from the images in the image index
func getImgLayerDigests(manifestsToInclude []ocispec.Descriptor, pkgPaths *layout.PackagePaths) ([]string, error) {
	var includeLayers []string
	for _, manifest := range manifestsToInclude {
		includeLayers = append(includeLayers, manifest.Digest.Hex()) // be sure to include image manifest
		manifestBytes, err := os.ReadFile(filepath.Join(pkgPaths.Images.Base, config.BlobsDir, manifest.Digest.Hex()))
		if err != nil {
			return nil, err
		}
		var imgManifest ocispec.Manifest
		err = goyaml.Unmarshal(manifestBytes, &imgManifest)
		if err != nil {
			return nil, err
		}
		includeLayers = append(includeLayers, imgManifest.Config.Digest.Hex()) // don't forget the config
		for _, layer := range imgManifest.Layers {
			includeLayers = append(includeLayers, layer.Digest.Hex())
		}
	}
	return includeLayers, nil
}

// filterPkgPaths grabs paths that either not in the blobs dir or are in includeLayers
func filterPkgPaths(pkgPaths *layout.PackagePaths, includeLayers []string, optionalComponents []v1alpha1.ZarfComponent) []string {
	var filteredPaths []string
	paths := pkgPaths.Files()
	for _, path := range paths {
		// include all paths that aren't in the blobs dir
		if !strings.Contains(path, config.BlobsDir) {
			// only grab req'd + specified optional components
			if strings.Contains(path, "/components/") {
				if shouldInclude := utils.IncludeComponent(path, optionalComponents); shouldInclude {
					filteredPaths = append(filteredPaths, path)
					continue
				}
			} else {
				filteredPaths = append(filteredPaths, path)
			}
		}
		// include paths that are in the blobs dir and are in includeLayers
		for _, layer := range includeLayers {
			if strings.Contains(path, config.BlobsDir) && strings.Contains(path, layer) {
				filteredPaths = append(filteredPaths, path)
				break
			}
		}
	}

	// ensure zarf.yaml, checksums and SBOMS (if exists) are always included
	// note we may have extra SBOMs because they are not filtered or modified
	alwaysInclude := []string{pkgPaths.ZarfYAML, pkgPaths.Checksums}
	if pkgPaths.SBOMs.Path != "" {
		alwaysInclude = append(alwaysInclude, pkgPaths.SBOMs.Path)
	}
	filteredPaths = helpers.MergeSlices(filteredPaths, alwaysInclude, func(a, b string) bool {
		return a == b
	})

	return filteredPaths
}
