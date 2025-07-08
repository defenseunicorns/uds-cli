// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/mholt/archives"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
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
func (tp *tarballBundleProvider) CreateBundleSBOM(extractSBOM bool, bundleName string) ([]string, error) {
	var warns []string
	rootManifest, err := tp.getBundleManifest()
	if err != nil {
		return warns, err
	}
	// make tmp dir for pkg SBOM extraction
	err = os.Mkdir(filepath.Join(tp.dst, config.BundleSBOM), 0o700)
	if err != nil {
		return warns, err
	}

	// track SBOM artifact paths, used for extraction and creation of bundleSBOM artifact
	SBOMArtifactPathMap := make(types.PathMap)

	for _, layer := range rootManifest.Layers {
		// get Zarf image manifests from bundle manifest
		if layer.Annotations[ocispec.AnnotationTitle] == config.BundleYAML {
			continue
		}

		// Open the tarball file for streaming instead of loading it all at once
		tarFile, err := os.Open(tp.src)
		if err != nil {
			return warns, err
		}

		var zarfImageManifest *oci.Manifest
		fileHandler := utils.ExtractJSON(&zarfImageManifest, filepath.Join(config.BlobsDir, layer.Digest.Encoded()))
		err = config.BundleArchiveFormat.Extract(context.TODO(), tarFile, fileHandler)
		tarFile.Close() // Close the file after extraction
		if err != nil {
			return warns, err
		}

		// find sbom layer descriptor and extract sbom tar from archive
		sbomDesc := zarfImageManifest.Locate(config.SBOMsTar)

		// if sbomDesc doesn't exist, continue
		if oci.IsEmptyDescriptor(sbomDesc) {
			continue
		}

		sbomFilePath := filepath.Join(config.BlobsDir, sbomDesc.Digest.Encoded())

		// check if file path already exists and remove
		// this fixes a bug where multiple pkgs have an empty SBOM tar archive
		if _, err := os.Stat(filepath.Join(tp.dst, sbomFilePath)); err == nil {
			err = os.Remove(filepath.Join(tp.dst, sbomFilePath))
			if err != nil {
				return warns, err
			}
		}

		// Open the tarball file again for streaming
		tarFile, err = os.Open(tp.src)
		if err != nil {
			return warns, err
		}

		fileHandler = utils.ExtractFile(sbomFilePath, tp.dst)
		err = config.BundleArchiveFormat.Extract(context.TODO(), tarFile, fileHandler)
		tarFile.Close() // Close the file after extraction
		if err != nil {
			return warns, err
		}

		// Open the extracted SBOM tar file for streaming
		sbomTarFile, err := os.Open(filepath.Join(tp.dst, sbomFilePath))
		if err != nil {
			return warns, err
		}

		extractor := utils.SBOMExtractor(tp.dst, SBOMArtifactPathMap)
		err = archives.Tar{}.Extract(context.TODO(), sbomTarFile, extractor)
		sbomTarFile.Close() // Close the file after extraction
		if err != nil {
			return warns, err
		}
	}

	return utils.HandleSBOM(extractSBOM, SBOMArtifactPathMap, bundleName, tp.dst)
}

func (tp *tarballBundleProvider) getBundleManifest() (*oci.Manifest, error) {
	if tp.rootManifest != nil {
		return tp.rootManifest, nil
	}
	return nil, errors.New("bundle root manifest not loaded")
}

// loadBundleManifest loads the bundle's root manifest and desc into the tarballBundleProvider so we don't have to load it multiple times
func (tp *tarballBundleProvider) loadBundleManifest() error {
	// Create a secure temporary directory for handling files
	secureTempDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	if err != nil {
		return fmt.Errorf("failed to create a secure temporary directory: %w", err)
	}
	defer os.RemoveAll(secureTempDir) // Ensure cleanup of the temp directory

	var index ocispec.Index

	// Open the tarball file for streaming
	tarFile, err := os.Open(tp.src)
	if err != nil {
		return err
	}
	defer tarFile.Close()

	fileHandler := utils.ExtractJSON(&index, "index.json")
	err = config.BundleArchiveFormat.Extract(context.TODO(), tarFile, fileHandler)
	if err != nil {
		return err
	}

	// local bundles only have one manifest entry in their index.json
	bundleManifestDesc := index.Manifests[0]
	tp.bundleRootDesc = bundleManifestDesc

	if len(index.Manifests) > 1 {
		return fmt.Errorf("expected only one manifest in index.json, found %d", len(index.Manifests))
	}

	manifestRelativePath := filepath.Join(config.BlobsDir, bundleManifestDesc.Digest.Encoded())
	manifestPath := filepath.Join(secureTempDir, manifestRelativePath)
	defer os.Remove(manifestPath)

	// Open the tarball file again for streaming
	tarFile, err = os.Open(tp.src)
	if err != nil {
		return err
	}
	defer tarFile.Close()

	fileHandler = utils.ExtractFile(manifestRelativePath, secureTempDir)
	err = config.BundleArchiveFormat.Extract(context.TODO(), tarFile, fileHandler)
	if err != nil {
		return err
	}

	if err := helpers.SHAsMatch(manifestPath, bundleManifestDesc.Digest.Encoded()); err != nil {
		return err
	}

	// Open the manifest file for streaming
	manifestFile, err := os.Open(manifestPath)
	if err != nil {
		return err
	}
	defer manifestFile.Close()

	var manifest *oci.Manifest
	decoder := json.NewDecoder(manifestFile)
	if err := decoder.Decode(&manifest); err != nil {
		return err
	}

	tp.rootManifest = manifest
	return nil
}

// LoadBundle loads a bundle from a tarball
func (tp *tarballBundleProvider) LoadBundle(_ types.BundlePullOptions, _ int) (*types.UDSBundle, types.PathMap, error) {
	return nil, nil, errors.New("uds pull does not support pulling local bundles")
}

// LoadBundleMetadata loads a bundle's metadata from a tarball
func (tp *tarballBundleProvider) LoadBundleMetadata() (types.PathMap, error) {
	bundleRootManifest, err := tp.getBundleManifest()
	if err != nil {
		return nil, err
	}
	pathsToExtract := config.BundleAlwaysPull

	filepaths := make(types.PathMap)

	for _, path := range pathsToExtract {
		layer := bundleRootManifest.Locate(path)
		if !oci.IsEmptyDescriptor(layer) {
			pathInTarball := filepath.Join(config.BlobsDir, layer.Digest.Encoded())
			abs := filepath.Join(tp.dst, pathInTarball)
			filepaths[path] = abs
			if !helpers.InvalidPath(abs) && helpers.SHAsMatch(abs, layer.Digest.Encoded()) == nil {
				continue
			}

			// Open the tarball file for streaming
			tarFile, err := os.Open(tp.src)
			if err != nil {
				return nil, err
			}

			fileHandler := utils.ExtractFile(pathInTarball, tp.dst)
			err = config.BundleArchiveFormat.Extract(context.TODO(), tarFile, fileHandler)
			tarFile.Close() // Close the file after extraction
			if err != nil {
				return nil, err
			}
		}
	}
	return filepaths, nil
}

// getZarfLayers returns the layers of the Zarf package that are in the bundle
func (tp *tarballBundleProvider) getZarfLayers(store *ocistore.Store, pkgManifestDesc ocispec.Descriptor) ([]ocispec.Descriptor, int64, error) {
	var layersToPull []ocispec.Descriptor
	estimatedPkgSize := int64(0)

	// Open the layer file for streaming
	layerFile, err := os.Open(filepath.Join(tp.dst, config.BlobsDir, pkgManifestDesc.Digest.Encoded()))
	if err != nil {
		return nil, int64(0), err
	}
	defer layerFile.Close()

	var zarfImageManifest *oci.Manifest
	decoder := json.NewDecoder(layerFile)
	if err := decoder.Decode(&zarfImageManifest); err != nil {
		return nil, int64(0), err
	}

	// only grab image layers that we want
	for _, layer := range zarfImageManifest.Manifest.Layers { //nolint:staticcheck
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
	defer progressBar.Close()
	remote.SetProgressWriter(progressBar)
	defer remote.ClearProgressWriter()

	srcRef := bundle.Metadata.Version
	// use tag given to remote e.g. ghcr.io/path/my-bundle:tag
	dstRef := remote.Repo().Reference.Reference

	// check for existing index
	index, err := boci.GetIndex(remote, srcRef)
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
		_, err = oras.Copy(tp.ctx, store, srcRef, remote.Repo(), dstRef, copyOpts)
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
