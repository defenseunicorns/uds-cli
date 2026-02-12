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
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	goyaml "github.com/goccy/go-yaml"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
)

func NewZarfOCIRemote(ctx context.Context, url string, platform ocispec.Platform, mods ...oci.Modifier) (*zoci.Remote, error) {
	modifiers := append([]oci.Modifier{
		oci.WithUserAgent("uds-cli/" + config.CLIVersion),
		oci.WithInsecureSkipVerify(config.CommonOptions.Insecure),
		oci.WithPlainHTTP(config.CommonOptions.Insecure),
	}, mods...)
	return zoci.NewRemote(ctx, url, platform, modifiers...)
}

// getImgLayerDigests grabs the digests of the layers from the images in the image index
func getImgLayerDigests(pkgDir string, manifestsToInclude []ocispec.Descriptor) ([]string, error) {
	var includeLayers []string
	for _, manifest := range manifestsToInclude {
		includeLayers = append(includeLayers, manifest.Digest.Hex()) // be sure to include image manifest
		manifestBytes, err := os.ReadFile(filepath.Join(pkgDir, layout.ImagesBlobsDir, manifest.Digest.Hex()))
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
func filterPkgPaths(pkgLayout *layout.PackageLayout, includeLayers []string, optionalComponents []v1alpha1.ZarfComponent) []string {
	var filteredPaths []string
	paths, err := pkgLayout.Files()
	if err != nil {
		return nil
	}
	for path := range paths {
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
	pkgDir := pkgLayout.DirPath()
	alwaysInclude := []string{filepath.Join(pkgDir, layout.ZarfYAML), filepath.Join(pkgDir, layout.Checksums)}
	if pkgLayout.ContainsSBOM() {
		alwaysInclude = append(alwaysInclude, filepath.Join(pkgDir, layout.SBOMTar))
	}
	filteredPaths = helpers.MergeSlices(filteredPaths, alwaysInclude, func(a, b string) bool {
		return a == b
	})

	return filteredPaths
}
