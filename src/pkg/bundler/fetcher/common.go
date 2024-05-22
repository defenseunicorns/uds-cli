// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package fetcher contains functionality to fetch local and remote Zarf pkgs for local bundling
package fetcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/layout"
	"github.com/defenseunicorns/zarf/src/pkg/packager/filters"
	zarfSources "github.com/defenseunicorns/zarf/src/pkg/packager/sources"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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

// loadPkg loads a package from a tarball source and filters out optional components
func loadPkg(pkgTmp string, pkgSrc zarfSources.PackageSource, optionalComponents []string) (zarfTypes.ZarfPackage, *layout.PackagePaths, error) {
	// create empty layout and source
	pkgPaths := layout.New(pkgTmp)

	// create filter for optional components
	createFilter := filters.Combine(
		filters.ForDeploy(strings.Join(optionalComponents, ","), false),
	)

	// load the package with the filter (calling LoadPackage populates the pkgPaths with the files from the tarball)
	pkg, _, err := pkgSrc.LoadPackage(pkgPaths, createFilter, false)
	if err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}
	return pkg, pkgPaths, nil
}

// filterImageIndex filters out optional components from the images index
func filterImageIndex(pkg zarfTypes.ZarfPackage, pathToImgIndex string) (ocispec.Index, error) {
	// read in images/index.json
	var imgIndex ocispec.Index
	var originalNumImgs int
	if pathToImgIndex != "" {
		indexBytes, err := os.ReadFile(pathToImgIndex)
		if err != nil {
			return ocispec.Index{}, err
		}
		err = json.Unmarshal(indexBytes, &imgIndex)
		if err != nil {
			return ocispec.Index{}, err
		}
		originalNumImgs = len(imgIndex.Manifests)
	}
	// include only images that are in the components using a map to dedup manifests
	manifestIncludeMap := map[string]ocispec.Descriptor{}
	for _, manifest := range imgIndex.Manifests {
		for _, component := range pkg.Components {
			for _, imgName := range component.Images {
				// include backwards compatibility shim for older Zarf versions that would leave docker.io off of image annotations
				if manifest.Annotations[ocispec.AnnotationBaseImageName] == imgName ||
					manifest.Annotations[ocispec.AnnotationBaseImageName] == fmt.Sprintf("docker.io/%s", imgName) {
					manifestIncludeMap[manifest.Digest.Hex()] = manifest
				}
			}
		}
	}
	// convert map to list and rewrite the index manifests
	var manifestsToInclude []ocispec.Descriptor
	for _, manifest := range manifestIncludeMap {
		manifestsToInclude = append(manifestsToInclude, manifest)
	}
	imgIndex.Manifests = manifestsToInclude

	// rewrite the images index
	if len(imgIndex.Manifests) > 0 && len(imgIndex.Manifests) != originalNumImgs {
		err := os.Remove(pathToImgIndex)
		if err != nil {
			return ocispec.Index{}, err
		}
		imgIndexBytes, err := json.Marshal(imgIndex)
		if err != nil {
			return ocispec.Index{}, err
		}
		err = os.WriteFile(pathToImgIndex, imgIndexBytes, 0600)
		if err != nil {
			return ocispec.Index{}, err
		}
	}

	return imgIndex, nil
}

// getImgLayerDigests grabs the digests of the layers from the images in the image index
func getImgLayerDigests(imgIndex ocispec.Index, pkgPaths *layout.PackagePaths) ([]string, error) {
	var includeLayers []string
	for _, manifest := range imgIndex.Manifests {
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
func filterPkgPaths(pkgPaths *layout.PackagePaths, includeLayers []string) ([]string, []string) {
	var filteredPaths []string
	var imageBlobs []string
	paths := pkgPaths.Files()
	for _, path := range paths {
		// include all paths that aren't in the blobs dir
		if !strings.Contains(path, config.BlobsDir) {
			filteredPaths = append(filteredPaths, path)
			continue
		}
		// include paths that are in the blobs dir and are in includeLayers
		for _, layer := range includeLayers {
			if strings.Contains(path, config.BlobsDir) && strings.Contains(path, layer) {
				filteredPaths = append(filteredPaths, path)
				imageBlobs = append(imageBlobs, path) // save off image blobs so we can rewrite pkgPaths (makes generating checksums easier)
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

	return filteredPaths, imageBlobs
}
