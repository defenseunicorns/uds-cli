// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package sources contains Zarf packager sources
package sources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/cache"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/layout"
	"github.com/defenseunicorns/zarf/src/pkg/packager/filters"
	"github.com/defenseunicorns/zarf/src/pkg/packager/sources"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
)

// RemoteBundle is a package source for remote bundles that implements Zarf's packager.PackageSource
type RemoteBundle struct {
	Pkg            types.Package
	PkgOpts        *zarfTypes.ZarfPackageOptions
	PkgManifestSHA string
	TmpDir         string
	Remote         *oci.OrasRemote
	nsOverrides    NamespaceOverrideMap
}

// LoadPackage loads a Zarf package from a remote bundle
func (r *RemoteBundle) LoadPackage(dst *layout.PackagePaths, filter filters.ComponentFilterStrategy, unarchiveAll bool) (zarfTypes.ZarfPackage, []string, error) {
	// todo: progress bar??
	layers, err := r.downloadPkgFromRemoteBundle()
	if err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}

	var pkg zarfTypes.ZarfPackage
	if err = utils.ReadYAMLStrict(dst.ZarfYAML, &pkg); err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}

	// if in dev mode and package is a zarf init config, return an empty package
	if config.Dev && pkg.Kind == zarfTypes.ZarfInitConfig {
		return zarfTypes.ZarfPackage{}, nil, nil
	}

	// filter pkg components and determine if its a partial pkg
	filteredComps, isPartialPkg, err := handleFilter(pkg, filter)
	if err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}
	pkg.Components = filteredComps

	dst.SetFromLayers(layers)

	err = sources.ValidatePackageIntegrity(dst, pkg.Metadata.AggregateChecksum, isPartialPkg)
	if err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}

	if unarchiveAll {
		for _, component := range pkg.Components {
			if err := dst.Components.Unarchive(component); err != nil {
				if layout.IsNotLoaded(err) {
					_, err := dst.Components.Create(component)
					if err != nil {
						return zarfTypes.ZarfPackage{}, nil, err
					}
				} else {
					return zarfTypes.ZarfPackage{}, nil, err
				}
			}
		}

		if dst.SBOMs.Path != "" {
			if err := dst.SBOMs.Unarchive(); err != nil {
				return zarfTypes.ZarfPackage{}, nil, err
			}
		}
	}
	addNamespaceOverrides(&pkg, r.nsOverrides)

	if config.Dev {
		setAsYOLO(&pkg)
	}

	// ensure we're using the correct package name as specified by the bundle
	pkg.Metadata.Name = r.Pkg.Name
	return pkg, nil, err
}

// LoadPackageMetadata loads a Zarf package's metadata from a remote bundle
func (r *RemoteBundle) LoadPackageMetadata(dst *layout.PackagePaths, _ bool, _ bool) (zarfTypes.ZarfPackage, []string, error) {
	ctx := context.TODO()
	root, err := r.Remote.FetchRoot(ctx)
	if err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}
	pkgManifestDesc := root.Locate(r.PkgManifestSHA)
	if oci.IsEmptyDescriptor(pkgManifestDesc) {
		return zarfTypes.ZarfPackage{}, nil, fmt.Errorf("zarf package %s with manifest sha %s not found", r.Pkg.Name, r.PkgManifestSHA)
	}

	// look at Zarf pkg manifest, grab zarf.yaml desc and download it
	pkgManifest, err := r.Remote.FetchManifest(ctx, pkgManifestDesc)
	if err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}

	var zarfYAMLDesc ocispec.Descriptor
	for _, layer := range pkgManifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == config.ZarfYAML {
			zarfYAMLDesc = layer
			break
		}
	}
	pkgBytes, err := r.Remote.FetchLayer(ctx, zarfYAMLDesc)
	if err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}
	var pkg zarfTypes.ZarfPackage
	if err = goyaml.Unmarshal(pkgBytes, &pkg); err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}
	err = zarfUtils.WriteYaml(filepath.Join(dst.Base, config.ZarfYAML), pkg, 0600)
	if err != nil {
		return zarfTypes.ZarfPackage{}, nil, err
	}

	// grab checksums.txt so we can validate pkg integrity
	var checksumLayer ocispec.Descriptor
	for _, layer := range pkgManifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == config.ChecksumsTxt {
			checksumBytes, err := r.Remote.FetchLayer(ctx, layer)
			if err != nil {
				return zarfTypes.ZarfPackage{}, nil, err
			}
			err = os.WriteFile(filepath.Join(dst.Base, config.ChecksumsTxt), checksumBytes, 0600)
			if err != nil {
				return zarfTypes.ZarfPackage{}, nil, err
			}
			checksumLayer = layer
			break
		}
	}

	dst.SetFromLayers([]ocispec.Descriptor{pkgManifestDesc, checksumLayer})

	err = sources.ValidatePackageIntegrity(dst, pkg.Metadata.AggregateChecksum, true)
	// ensure we're using the correct package name as specified by the bundle
	pkg.Metadata.Name = r.Pkg.Name
	return pkg, nil, err
}

// Collect doesn't need to be implemented
func (r *RemoteBundle) Collect(_ string) (string, error) {
	return "", fmt.Errorf("not implemented in %T", r)
}

// downloadPkgFromRemoteBundle downloads a Zarf package from a remote bundle
func (r *RemoteBundle) downloadPkgFromRemoteBundle() ([]ocispec.Descriptor, error) {
	ctx := context.TODO()
	rootManifest, err := r.Remote.FetchRoot(ctx)
	if err != nil {
		return nil, err
	}

	pkgManifestDesc := rootManifest.Locate(r.PkgManifestSHA)
	if oci.IsEmptyDescriptor(pkgManifestDesc) {
		return nil, fmt.Errorf("package %s does not exist in this bundle", r.PkgManifestSHA)
	}
	// hack Zarf media type so that FetchManifest works
	pkgManifestDesc.MediaType = zoci.ZarfLayerMediaTypeBlob
	pkgManifest, err := r.Remote.FetchManifest(ctx, pkgManifestDesc)
	if err != nil || pkgManifest == nil {
		return nil, err
	}

	estimatedBytes := int64(0)
	layersToPull := []ocispec.Descriptor{pkgManifestDesc}
	layersInBundle := []ocispec.Descriptor{pkgManifestDesc}

	// get pkg layers that we want to pull
	pkgLayers, _, err := boci.FindBundledPkgLayers(ctx, r.Pkg, rootManifest, r.Remote)
	if err != nil {
		return nil, err
	}

	// todo: we seem to need to specifically pull the layers from the pkgManifest here, but not in the
	// other location that FindBundledPkgLayers is called. Why is that?
	// I believe it's bc here we are going to iterate through those layers and fill out a layout with
	// the annotations from each desc (only pkgManifest layers contain the necessary annotations)

	// correlate descs in pkg root manifest with the pkg layers to pull
	for _, manifestLayer := range pkgManifest.Layers {
		for _, pkgLayer := range pkgLayers {
			if pkgLayer.Digest.Encoded() == manifestLayer.Digest.Encoded() {
				layersInBundle = append(layersInBundle, manifestLayer)
				digest := manifestLayer.Digest.Encoded()

				// if it's an image layer and is in the cache, use it
				if strings.Contains(manifestLayer.Annotations[ocispec.AnnotationTitle], config.BlobsDir) && cache.Exists(digest) {
					dst := filepath.Join(r.TmpDir, "images", config.BlobsDir)
					err = cache.Use(digest, dst)
					if err != nil {
						return nil, err
					}
				} else {
					// not in cache, so pull
					layersToPull = append(layersToPull, manifestLayer)
					estimatedBytes += manifestLayer.Size
				}
				break // if layer is found, break out of inner loop
			}
		}
	}

	store, err := file.New(r.TmpDir)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	// copy zarf pkg to local store
	copyOpts := boci.CreateCopyOpts(layersToPull, config.CommonOptions.OCIConcurrency)
	doneSaving := make(chan error)
	go zarfUtils.RenderProgressBarForLocalDirWrite(r.TmpDir, estimatedBytes, doneSaving, fmt.Sprintf("Pulling bundled Zarf pkg: %s", r.Pkg.Name), fmt.Sprintf("Successfully pulled package: %s", r.Pkg.Name))

	_, err = oras.Copy(ctx, r.Remote.Repo(), r.Remote.Repo().Reference.String(), store, "", copyOpts)
	doneSaving <- err
	<-doneSaving
	if err != nil {
		return nil, err
	}

	return layersInBundle, nil
}
