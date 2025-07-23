// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

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
	goyaml "github.com/goccy/go-yaml"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
	"oras.land/oras-go/v2/content/file"
)

// RemoteBundle is a package source for remote bundles that implements Zarf's packager.PackageSource
type RemoteBundle struct {
	Pkg                     types.Package
	PkgOpts                 *zarfTypes.ZarfPackageOptions
	PkgManifestSHA          string
	TmpDir                  string
	PublicKeyPath           string
	Remote                  *oci.OrasRemote
	nsOverrides             NamespaceOverrideMap
	bundleCfg               types.BundleConfig
	SkipSignatureValidation bool
}

// LoadPackage loads a Zarf package from a remote bundle
func (r *RemoteBundle) LoadPackage(ctx context.Context, filter filters.ComponentFilterStrategy) (*layout.PackageLayout, []string, error) {
	// todo: progress bar??
	var err error

	if config.Dev {
		if _, ok := r.bundleCfg.DevDeployOpts.Ref[r.Pkg.Name]; ok {
			// create new oras remote for package
			platform := ocispec.Platform{
				Architecture: config.GetArch(),
				OS:           oci.MultiOS,
			}
			// get remote client
			repoUrl := fmt.Sprintf("%s:%s", r.Pkg.Repository, r.Pkg.Ref)
			remote, _ := zoci.NewRemote(ctx, repoUrl, platform)
			_, err = remote.PullPackage(ctx, r.TmpDir, config.CommonOptions.OCIConcurrency)
		} else {
			_, err = r.downloadPkgFromRemoteBundle()
		}
	} else {
		_, err = r.downloadPkgFromRemoteBundle()
	}

	if err != nil {
		return nil, nil, err
	}

	var pkg v1alpha1.ZarfPackage
	if err = utils.ReadYAMLStrict(filepath.Join(r.TmpDir, layout.ZarfYAML), &pkg); err != nil {
		return nil, nil, err
	}

	// if in dev mode and package is a zarf init config, return an empty package
	if config.Dev && pkg.Kind == v1alpha1.ZarfInitConfig {
		return nil, nil, nil
	}

	// filter pkg components and determine if its a partial pkg
	filteredComps, isPartialPkg, err := handleFilter(pkg, filter)
	if err != nil {
		return nil, nil, err
	}
	pkg.Components = filteredComps

	layoutOpts := layout.PackageLayoutOptions{
		PublicKeyPath:           r.PublicKeyPath,
		SkipSignatureValidation: r.SkipSignatureValidation,
		IsPartial:               isPartialPkg,
		Filter:                  filter,
	}

	pkgLayout, err := layout.LoadFromDir(ctx, r.TmpDir, layoutOpts)
	if err != nil {
		return nil, nil, err
	}

	addNamespaceOverrides(&pkgLayout.Pkg, r.nsOverrides)

	if config.Dev {
		setAsYOLO(&pkgLayout.Pkg)
	}

	// ensure we're using the correct package name as specified by the bundle
	pkgLayout.Pkg.Metadata.Name = r.Pkg.Name
	return pkgLayout, nil, err
}

// LoadPackageMetadata loads a Zarf package's metadata from a remote bundle
func (r *RemoteBundle) LoadPackageMetadata(ctx context.Context, _ bool, _ bool) (v1alpha1.ZarfPackage, []string, error) {
	root, err := r.Remote.FetchRoot(ctx)
	if err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}
	pkgManifestDesc := root.Locate(r.PkgManifestSHA)
	if oci.IsEmptyDescriptor(pkgManifestDesc) {
		return v1alpha1.ZarfPackage{}, nil, fmt.Errorf("zarf package %s with manifest sha %s not found", r.Pkg.Name, r.PkgManifestSHA)
	}

	// look at Zarf pkg manifest, grab zarf.yaml desc and download it
	pkgManifest, err := r.Remote.FetchManifest(ctx, pkgManifestDesc)
	if err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
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
		return v1alpha1.ZarfPackage{}, nil, err
	}
	var pkg v1alpha1.ZarfPackage
	if err = goyaml.Unmarshal(pkgBytes, &pkg); err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}
	err = zarfUtils.WriteYaml(filepath.Join(r.TmpDir, layout.ZarfYAML), pkg, 0600)
	if err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}

	// grab checksums.txt so we can validate pkg integrity
	for _, layer := range pkgManifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == config.ChecksumsTxt {
			checksumBytes, err := r.Remote.FetchLayer(ctx, layer)
			if err != nil {
				return v1alpha1.ZarfPackage{}, nil, err
			}
			err = os.WriteFile(filepath.Join(r.TmpDir, layout.Checksums), checksumBytes, 0600)
			if err != nil {
				return v1alpha1.ZarfPackage{}, nil, err
			}
			break
		}
	}

	// ensure we're using the correct package name as specified by the bundle
	pkg.Metadata.Name = r.Pkg.Name
	return pkg, nil, err
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

	// create local file target for pkg layers
	target, err := file.New(r.TmpDir)
	if err != nil {
		return nil, err
	}
	defer target.Close()
	_, err = boci.CopyLayers(layersToPull, estimatedBytes, r.TmpDir, r.Remote.Repo(), target, r.Pkg.Name)
	if err != nil {
		return nil, err
	}

	return layersInBundle, nil
}
