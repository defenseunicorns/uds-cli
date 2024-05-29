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

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	av3 "github.com/mholt/archiver/v3"
	av4 "github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	ocistore "oras.land/oras-go/v2/content/oci"
)

type tarballBundleProvider struct {
	ctx context.Context
	src string
	dst string

	// these fields are populated by loadBundleManifest as part of the provider constructor
	bundleRootDesc ocispec.Descriptor
	rootManifest   *oci.Manifest
}

// CreateBundleSBOM creates a bundle-level SBOM from the underlying Zarf packages, if the Zarf package contains an SBOM
func (tp *tarballBundleProvider) CreateBundleSBOM(extractSBOM bool) error {
	rootManifest, err := tp.getBundleManifest()
	if err != nil {
		return err
	}
	// make tmp dir for pkg SBOM extraction
	err = os.Mkdir(filepath.Join(tp.dst, config.BundleSBOM), 0o700)
	if err != nil {
		return err
	}
	SBOMArtifactPathMap := make(types.PathMap)
	containsSBOMs := false

	for _, layer := range rootManifest.Layers {
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
		var zarfImageManifest *oci.Manifest
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

func (tp *tarballBundleProvider) getBundleManifest() (*oci.Manifest, error) {
	if tp.rootManifest != nil {
		return tp.rootManifest, nil
	}
	return nil, fmt.Errorf("bundle root manifest not loaded")
}

// loadBundleManifest loads the bundle's root manifest and desc into the tarballBundleProvider so we don't have to load it multiple times
func (tp *tarballBundleProvider) loadBundleManifest() error {
	// Create a secure temporary directory for handling files
	secureTempDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	if err != nil {
		return fmt.Errorf("failed to create a secure temporary directory: %w", err)
	}
	defer os.RemoveAll(secureTempDir) // Ensure cleanup of the temp directory

	if err := av3.Extract(tp.src, "index.json", secureTempDir); err != nil {
		return fmt.Errorf("failed to extract index.json from %s: %w", tp.src, err)
	}
	indexPath := filepath.Join(secureTempDir, "index.json")

	defer os.Remove(indexPath)

	b, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("failed to read index.json: %w", err)
	}

	var index ocispec.Index
	if err := json.Unmarshal(b, &index); err != nil {
		return fmt.Errorf("failed to unmarshal index.json: %w", err)
	}
	// local bundles only have one manifest entry in their index.json
	bundleManifestDesc := index.Manifests[0]
	tp.bundleRootDesc = bundleManifestDesc

	if len(index.Manifests) > 1 {
		return fmt.Errorf("expected only one manifest in index.json, found %d", len(index.Manifests))
	}

	manifestRelativePath := filepath.Join(config.BlobsDir, bundleManifestDesc.Digest.Encoded())

	if err := av3.Extract(tp.src, manifestRelativePath, secureTempDir); err != nil {
		return fmt.Errorf("failed to extract %s from %s: %w", bundleManifestDesc.Digest.Encoded(), tp.src, err)
	}

	manifestPath := filepath.Join(secureTempDir, manifestRelativePath)

	defer os.Remove(manifestPath)

	if err := helpers.SHAsMatch(manifestPath, bundleManifestDesc.Digest.Encoded()); err != nil {
		return err
	}

	b, err = os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	var manifest *oci.Manifest

	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}

	tp.rootManifest = manifest
	return nil
}

// LoadBundle loads a bundle from a tarball
func (tp *tarballBundleProvider) LoadBundle(_ types.BundlePullOptions, _ int) (*types.UDSBundle, types.PathMap, error) {
	return nil, nil, fmt.Errorf("uds pull does not support pulling local bundles")
}

// LoadBundleMetadata loads a bundle's metadata from a tarball
func (tp *tarballBundleProvider) LoadBundleMetadata() (types.PathMap, error) {
	bundleRootManifest, err := tp.getBundleManifest()
	if err != nil {
		return nil, err
	}
	pathsToExtract := config.BundleAlwaysPull

	loaded := make(types.PathMap)

	for _, path := range pathsToExtract {
		layer := bundleRootManifest.Locate(path)
		if !oci.IsEmptyDescriptor(layer) {
			pathInTarball := filepath.Join(config.BlobsDir, layer.Digest.Encoded())
			abs := filepath.Join(tp.dst, pathInTarball)
			loaded[path] = abs
			if !helpers.InvalidPath(abs) && helpers.SHAsMatch(abs, layer.Digest.Encoded()) == nil {
				continue
			}
			if err := av3.Extract(tp.src, pathInTarball, tp.dst); err != nil {
				return nil, fmt.Errorf("failed to extract %s from %s: %w", path, tp.src, err)
			}
		}
	}
	return loaded, nil
}

// getZarfLayers returns the layers of the Zarf package that are in the bundle
func (tp *tarballBundleProvider) getZarfLayers(store *ocistore.Store, pkgManifestDesc ocispec.Descriptor) ([]ocispec.Descriptor, int64, error) {
	var layersToPull []ocispec.Descriptor
	estimatedPkgSize := int64(0)

	layerBytes, err := os.ReadFile(filepath.Join(tp.dst, config.BlobsDir, pkgManifestDesc.Digest.Encoded()))
	if err != nil {
		return nil, int64(0), err
	}

	var zarfImageManifest *oci.Manifest
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
	bundleRootManifest, err := tp.getBundleManifest()
	if err != nil {
		return err
	}
	estimatedBytes := int64(0)

	// reference local store holding untarred bundle
	store, err := ocistore.NewWithContext(tp.ctx, tp.dst)
	if err != nil {
		return err
	}
	// push bundle layers to remote
	for _, manifestDesc := range bundleRootManifest.Layers {
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
	layersToPush = append(layersToPush, bundleRootManifest.Config)

	// copy bundle
	copyOpts := boci.CreateCopyOpts(layersToPush, config.CommonOptions.OCIConcurrency)
	progressBar := message.NewProgressBar(estimatedBytes, fmt.Sprintf("Publishing %s:%s", remote.Repo().Reference.Repository, remote.Repo().Reference.Reference))
	defer progressBar.Stop()
	remote.SetProgressWriter(progressBar)
	defer remote.ClearProgressWriter()

	ref := bundle.Metadata.Version

	// check for existing index
	index, err := boci.GetIndex(remote, ref)
	if err != nil {
		return err
	}

	// copy bundle layers to remote with retries
	maxRetries := 3
	retries := 0

	// reset retries if a desc was successful
	copyOpts.PostCopy = func(_ context.Context, _ ocispec.Descriptor) error {
		retries = 0
		return nil
	}

	for {
		_, err = oras.Copy(tp.ctx, store, ref, remote.Repo(), ref, copyOpts)
		if err != nil && retries < maxRetries {
			retries++
			message.Debugf("Encountered err during publish: %s\nRetrying %d/%d", err, retries, maxRetries)
			continue
		} else if err != nil {
			return err
		}
		break
	}

	// create or update, then push index.json
	err = boci.UpdateIndex(index, remote, &bundle, tp.bundleRootDesc)
	if err != nil {
		return err
	}

	progressBar.Successf("Published %s", remote.Repo().Reference)
	return nil
}
