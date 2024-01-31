// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	av3 "github.com/mholt/archiver/v3"
	av4 "github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	ocistore "oras.land/oras-go/v2/content/oci"
)

type tarballBundleProvider struct {
	ctx                context.Context
	src                string
	dst                string
	bundleRootManifest *oci.ZarfOCIManifest
	bundleRootDesc     ocispec.Descriptor
}

// CreateBundleSBOM creates a bundle-level SBOM from the underlying Zarf packages, if the Zarf package contains an SBOM
func (tp *tarballBundleProvider) CreateBundleSBOM(extractSBOM bool) error {
	err := tp.getBundleManifest()
	if err != nil {
		return err
	}
	// make tmp dir for pkg SBOM extraction
	err = os.Mkdir(filepath.Join(tp.dst, config.BundleSBOM), 0700)
	if err != nil {
		return err
	}
	SBOMArtifactPathMap := make(PathMap)
	containsSBOMs := false

	for _, layer := range tp.bundleRootManifest.Layers {
		// get Zarf image manifests from bundle manifest
		if layer.Annotations[ocispec.AnnotationTitle] == config.BundleYAML {
			continue
		}
		layerFilePath := filepath.Join(config.BlobsDir, layer.Digest.Encoded())
		if err := av3.Extract(tp.src, layerFilePath, tp.dst); err != nil {
			return fmt.Errorf("failed to extract %s from %s: %w", layer.Digest.Encoded(), tp.src, err)
		}

		// read in and unmarshal Zarf image manifest
		zarfManifestBytes, err := os.ReadFile(filepath.Join(tp.dst, layerFilePath))
		if err != nil {
			return err
		}
		var zarfImageManifest *oci.ZarfOCIManifest
		if err := json.Unmarshal(zarfManifestBytes, &zarfImageManifest); err != nil {
			return err
		}

		// find sbom layer descriptor and extract sbom tar from archive
		sbomDesc := zarfImageManifest.Locate(config.SBOMsTar)

		// if sbomDesc doesn't exist, continue
		if oci.IsEmptyDescriptor(sbomDesc) {
			message.Warnf("%s not found in Zarf pkg", config.SBOMsTar)
			continue
		}

		sbomFilePath := filepath.Join(config.BlobsDir, sbomDesc.Digest.Encoded())
		if err := av3.Extract(tp.src, sbomFilePath, tp.dst); err != nil {
			return fmt.Errorf("failed to extract %s from %s: %w", layer.Digest.Encoded(), tp.src, err)
		}
		sbomTarBytes, err := os.ReadFile(filepath.Join(tp.dst, sbomFilePath))
		if err != nil {
			return err
		}
		extractor := utils.SBOMExtractor(tp.dst, SBOMArtifactPathMap)
		err = av4.Tar{}.Extract(context.TODO(), bytes.NewReader(sbomTarBytes), nil, extractor)
		if err != nil {
			return err
		}
		containsSBOMs = true
	}
	if extractSBOM {
		if !containsSBOMs {
			message.Warnf("Cannot extract, no SBOMs found in bundle")
			return nil
		}
		currentDir, err := os.Getwd()
		if err != nil {
			return err
		}
		err = utils.MoveExtractedSBOMs(tp.dst, currentDir)
		if err != nil {
			return err
		}
	} else {
		err = utils.CreateSBOMArtifact(SBOMArtifactPathMap)
		if err != nil {
			return err
		}
	}
	return nil
}

func (tp *tarballBundleProvider) getBundleManifest() error {
	if tp.bundleRootManifest != nil {
		return nil
	}

	if err := av3.Extract(tp.src, "index.json", tp.dst); err != nil {
		return fmt.Errorf("failed to extract index.json from %s: %w", tp.src, err)
	}

	indexPath := filepath.Join(tp.dst, "index.json")

	defer os.Remove(indexPath)

	b, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	var index ocispec.Index

	if err := json.Unmarshal(b, &index); err != nil {
		return err
	}

	// local bundles only have one manifest entry in their index.json
	bundleManifestDesc := index.Manifests[0]
	tp.bundleRootDesc = bundleManifestDesc

	if len(index.Manifests) > 1 {
		return fmt.Errorf("expected only one manifest in index.json, found %d", len(index.Manifests))
	}

	manifestRelativePath := filepath.Join(config.BlobsDir, bundleManifestDesc.Digest.Encoded())

	if err := av3.Extract(tp.src, manifestRelativePath, tp.dst); err != nil {
		return fmt.Errorf("failed to extract %s from %s: %w", bundleManifestDesc.Digest.Encoded(), tp.src, err)
	}

	manifestPath := filepath.Join(tp.dst, manifestRelativePath)

	defer os.Remove(manifestPath)

	if err := zarfUtils.SHAsMatch(manifestPath, bundleManifestDesc.Digest.Encoded()); err != nil {
		return err
	}

	b, err = os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	var manifest *oci.ZarfOCIManifest

	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}

	tp.bundleRootManifest = manifest
	return nil
}

// LoadBundle loads a bundle from a tarball
func (tp *tarballBundleProvider) LoadBundle(_ int) (PathMap, error) {
	return nil, fmt.Errorf("uds pull does not support pulling local bundles")
}

// LoadBundleMetadata loads a bundle's metadata from a tarball
func (tp *tarballBundleProvider) LoadBundleMetadata() (PathMap, error) {
	if err := tp.getBundleManifest(); err != nil {
		return nil, err
	}
	pathsToExtract := config.BundleAlwaysPull

	loaded := make(PathMap)

	for _, path := range pathsToExtract {
		layer := tp.bundleRootManifest.Locate(path)
		if !oci.IsEmptyDescriptor(layer) {
			pathInTarball := filepath.Join(config.BlobsDir, layer.Digest.Encoded())
			abs := filepath.Join(tp.dst, pathInTarball)
			loaded[path] = abs
			if !zarfUtils.InvalidPath(abs) && zarfUtils.SHAsMatch(abs, layer.Digest.Encoded()) == nil {
				continue
			}
			if err := av3.Extract(tp.src, pathInTarball, tp.dst); err != nil {
				return nil, fmt.Errorf("failed to extract %s from %s: %w", path, tp.src, err)
			}
		}
	}
	return loaded, nil
}

func (tp *tarballBundleProvider) getZarfLayers(store *ocistore.Store, pkgManifestDesc ocispec.Descriptor) ([]ocispec.Descriptor, int64, error) {
	var layersToPull []ocispec.Descriptor
	estimatedPkgSize := int64(0)

	layerBytes, err := os.ReadFile(filepath.Join(tp.dst, config.BlobsDir, pkgManifestDesc.Digest.Encoded()))
	if err != nil {
		return nil, int64(0), err
	}

	var zarfImageManifest *oci.ZarfOCIManifest
	if err := json.Unmarshal(layerBytes, &zarfImageManifest); err != nil {
		return nil, int64(0), err
	}

	// only grab image layers that we want
	for _, layer := range zarfImageManifest.Manifest.Layers {
		ok, err := store.Exists(tp.ctx, layer)
		if err != nil {
			return nil, int64(0), err
		}
		if ok {
			estimatedPkgSize += layer.Size
			layersToPull = append(layersToPull, layer)
		}
	}

	return layersToPull, estimatedPkgSize, nil
}

// PublishBundle publishes a local bundle to a remote OCI registry
func (tp *tarballBundleProvider) PublishBundle(bundle types.UDSBundle, remote *oci.OrasRemote) error {
	var layersToPush []ocispec.Descriptor
	if err := tp.getBundleManifest(); err != nil {
		return err
	}
	estimatedBytes := int64(0)

	// reference local store holding untarred bundle
	store, err := ocistore.NewWithContext(tp.ctx, tp.dst)
	if err != nil {
		return err
	}
	// push bundle layers to remote
	for _, manifestDesc := range tp.bundleRootManifest.Layers {
		layersToPush = append(layersToPush, manifestDesc)
		if manifestDesc.Annotations[ocispec.AnnotationTitle] == config.BundleYAML {
			continue // uds-bundle.yaml doesn't have layers
		}
		layers, estimatedPkgSize, err := tp.getZarfLayers(store, manifestDesc)
		estimatedBytes += estimatedPkgSize
		if err != nil {
			return err
		}
		layersToPush = append(layersToPush, layers...)
	}

	// grab image config
	layersToPush = append(layersToPush, tp.bundleRootManifest.Config)

	// copy bundle
	copyOpts := utils.CreateCopyOpts(layersToPush, config.CommonOptions.OCIConcurrency)
	if err != nil {
		return err
	}
	remote.Transport.ProgressBar = message.NewProgressBar(estimatedBytes, fmt.Sprintf("Publishing %s:%s", remote.Repo().Reference.Repository, remote.Repo().Reference.Reference))
	defer remote.Transport.ProgressBar.Stop()

	ref := bundle.Metadata.Version

	// check for existing index
	index, err := utils.GetIndex(remote, ref)
	if err != nil {
		return err
	}

	_, err = oras.Copy(tp.ctx, store, ref, remote.Repo(), ref, copyOpts)
	if err != nil {
		return err
	}

	// create or update, then push index.json
	err = utils.UpdateIndex(index, remote, bundle, tp.bundleRootDesc)
	if err != nil {
		return err
	}

	remote.Transport.ProgressBar.Successf("Published %s", remote.Repo().Reference)
	return nil
}

// ZarfPackageNameMap gets zarf package name mappings from tarball provider
func (tp *tarballBundleProvider) ZarfPackageNameMap() (map[string]string, error) {
	if err := tp.getBundleManifest(); err != nil {
		return nil, err
	}

	nameMap := make(map[string]string)
	for _, layer := range tp.bundleRootManifest.Layers {
		if layer.MediaType == oci.ZarfLayerMediaTypeBlob {
			// only the uds bundle layer will have AnnotationTitle set
			if layer.Annotations[ocispec.AnnotationTitle] != config.BundleYAML {
				nameMap[layer.Annotations[config.UDSPackageNameAnnotation]] = layer.Annotations[config.ZarfPackageNameAnnotation]
			}
		}
	}
	return nameMap, nil
}
