// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package sources contains Zarf packager sources
package sources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/zarf/src/pkg/layout"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/packager/sources"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/file"

	"github.com/defenseunicorns/uds-cli/src/config"
)

// RemoteBundle is a package source for remote bundles that implements Zarf's packager.PackageSource
type RemoteBundle struct {
	PkgName        string
	PkgOpts        *zarfTypes.ZarfPackageOptions
	PkgManifestSHA string
	TmpDir         string
	Remote         *oci.OrasRemote
	isPartial      bool
}

// LoadPackage loads a Zarf package from a remote bundle
func (r *RemoteBundle) LoadPackage(dst *layout.PackagePaths, unarchiveAll bool) error {
	packageSpinner := message.NewProgressSpinner("Loading bundled Zarf package: %s", r.PkgName)
	defer packageSpinner.Stop()
	layers, err := r.downloadPkgFromRemoteBundle()
	if err != nil {
		return err
	}

	var pkg zarfTypes.ZarfPackage
	if err = utils.ReadYaml(dst.ZarfYAML, &pkg); err != nil {
		return err
	}

	dst.SetFromLayers(layers)

	err = sources.ValidatePackageIntegrity(dst, pkg.Metadata.AggregateChecksum, r.isPartial)
	if err != nil {
		return err
	}

	if unarchiveAll {
		for _, component := range pkg.Components {
			if err := dst.Components.Unarchive(component); err != nil {
				if layout.IsNotLoaded(err) {
					_, err := dst.Components.Create(component)
					if err != nil {
						return err
					}
				} else {
					return err
				}
			}
		}

		if dst.SBOMs.Path != "" {
			if err := dst.SBOMs.Unarchive(); err != nil {
				return err
			}
		}
	}

	packageSpinner.Successf("Loaded bundled Zarf package: %s", r.PkgName)
	return nil
}

// LoadPackageMetadata loads a Zarf package's metadata from a remote bundle
func (r *RemoteBundle) LoadPackageMetadata(dst *layout.PackagePaths, _ bool, _ bool) (err error) {
	root, err := r.Remote.FetchRoot()
	if err != nil {
		return err
	}
	pkgManifestDesc := root.Locate(r.PkgManifestSHA)
	if oci.IsEmptyDescriptor(pkgManifestDesc) {
		return fmt.Errorf("zarf package %s with manifest sha %s not found", r.PkgName, r.PkgManifestSHA)
	}

	// look at Zarf pkg manifest, grab zarf.yaml desc and download it
	pkgManifest, err := r.Remote.FetchManifest(pkgManifestDesc)
	var zarfYAMLDesc ocispec.Descriptor
	for _, layer := range pkgManifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == config.ZarfYAML {
			zarfYAMLDesc = layer
			break
		}
	}
	zarfYAMLBytes, err := r.Remote.FetchLayer(zarfYAMLDesc)
	if err != nil {
		return err
	}
	var zarfYAML zarfTypes.ZarfPackage
	if err = goyaml.Unmarshal(zarfYAMLBytes, &zarfYAML); err != nil {
		return err
	}
	err = utils.WriteYaml(filepath.Join(dst.Base, config.ZarfYAML), zarfYAML, 0644)

	// grab checksums.txt so we can validate pkg integrity
	var checksumLayer ocispec.Descriptor
	for _, layer := range pkgManifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == config.ChecksumsTxt {
			checksumBytes, err := r.Remote.FetchLayer(layer)
			if err != nil {
				return err
			}
			err = os.WriteFile(filepath.Join(dst.Base, config.ChecksumsTxt), checksumBytes, 0644)
			if err != nil {
				return err
			}
			checksumLayer = layer
			break
		}
	}

	dst.SetFromLayers([]ocispec.Descriptor{pkgManifestDesc, checksumLayer})

	err = sources.ValidatePackageIntegrity(dst, zarfYAML.Metadata.AggregateChecksum, true)
	return err
}

// Collect doesn't need to be implemented
func (r *RemoteBundle) Collect(_ string) (string, error) {
	return "", fmt.Errorf("not implemented in %T", r)
}

// downloadPkgFromRemoteBundle downloads a Zarf package from a remote bundle
func (r *RemoteBundle) downloadPkgFromRemoteBundle() ([]ocispec.Descriptor, error) {
	// todo: use oras.Copy for faster downloads
	rootManifest, err := r.Remote.FetchRoot()
	if err != nil {
		return nil, err
	}

	pkgManifestDesc := rootManifest.Locate(r.PkgManifestSHA)
	if oci.IsEmptyDescriptor(pkgManifestDesc) {
		return nil, fmt.Errorf("package %s does not exist in this bundle", r.PkgManifestSHA)
	}
	// hack to Zarf media type so that FetchManifest works
	pkgManifestDesc.MediaType = oci.ZarfLayerMediaTypeBlob
	pkgManifest, err := r.Remote.FetchManifest(pkgManifestDesc)
	if err != nil || pkgManifest == nil {
		return nil, err
	}

	// including the package manifest uses some ORAs FindSuccessors hackery to expand the manifest into all layers
	// as oras.Copy was designed for resolving layers via a manifest reference, not a manifest embedded inside of another
	// image
	layersToPull := []ocispec.Descriptor{pkgManifestDesc}
	for _, layer := range pkgManifest.Layers {
		// only fetch layers that exist
		// since optional-components exists, there will be layers that don't exist
		// as the package's preserved manifest will contain all layers for all components
		ok, _ := r.Remote.Repo().Blobs().Exists(context.TODO(), layer)
		if ok {
			layersToPull = append(layersToPull, layer)
		}
	}

	store, err := file.New(r.TmpDir)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	spinner := message.NewProgressSpinner("Pulling bundled Zarf package")
	defer spinner.Stop()
	for _, layer := range layersToPull {
		spinner.Updatef(fmt.Sprintf("Pulling bundle layer: %s", layer.Digest.Encoded()))
		lb, err := r.Remote.Repo().Fetch(context.TODO(), layer)
		if err != nil {
			return nil, err
		}

		err = store.Push(context.TODO(), layer, lb)
		if err != nil {
			return nil, err
		}
	}
	if len(pkgManifest.Layers) > len(layersToPull) {
		r.isPartial = true
	}

	spinner.Successf("Package pull successful")
	return layersToPull, nil
}
