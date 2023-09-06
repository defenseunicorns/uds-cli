// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"io"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/zarf/src/pkg/oci"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"
	av3 "github.com/mholt/archiver/v3"
	av4 "github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	ocistore "oras.land/oras-go/v2/content/oci"
)

type tarballBundleProvider struct {
	ctx      context.Context
	src      string
	dst      string
	manifest *oci.ZarfOCIManifest
}

func extractJSON(j any) func(context.Context, av4.File) error {
	return func(_ context.Context, file av4.File) error {
		stream, err := file.Open()
		if err != nil {
			return err
		}
		defer stream.Close()

		bytes, err := io.ReadAll(stream)
		if err != nil {
			return err
		}
		return json.Unmarshal(bytes, &j)
	}
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

	for _, layer := range tp.manifest.Layers {
		// get Zarf image manifests from bundle manifest
		if len(layer.Annotations) != 0 {
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
	}
	if extractSBOM {
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
	if tp.manifest != nil {
		return nil
	}

	if err := av3.Extract(tp.src, "index.json", tp.dst); err != nil {
		return fmt.Errorf("failed to extract index.json from %s: %w", tp.src, err)
	}

	indexPath := filepath.Join(tp.dst, "index.json")

	defer os.Remove(indexPath)

	bytes, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	var index ocispec.Index

	if err := json.Unmarshal(bytes, &index); err != nil {
		return err
	}

	// due to logic during the bundle pull process, this index.json should only have one manifest
	bundleManifestDesc := index.Manifests[0]

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

	bytes, err = os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	var manifest *oci.ZarfOCIManifest

	if err := json.Unmarshal(bytes, &manifest); err != nil {
		return err
	}

	tp.manifest = manifest
	return nil
}

// LoadBundle loads a bundle from a tarball
func (tp *tarballBundleProvider) LoadBundle(_ int) (PathMap, error) {
	loaded := make(PathMap)

	if err := tp.getBundleManifest(); err != nil {
		return nil, err
	}

	store, err := ocistore.NewWithContext(tp.ctx, tp.dst)
	if err != nil {
		return nil, err
	}

	layersToExtract := []ocispec.Descriptor{}

	format := av4.CompressedArchive{
		Compression: av4.Zstd{},
		Archival:    av4.Tar{},
	}

	sourceArchive, err := os.Open(tp.src)
	if err != nil {
		return nil, err
	}

	defer sourceArchive.Close()

	for _, layer := range tp.manifest.Layers {
		if layer.MediaType == ocispec.MediaTypeImageManifest {
			var manifest oci.ZarfOCIManifest
			if err := format.Extract(tp.ctx, sourceArchive, []string{filepath.Join(config.BlobsDir, layer.Digest.Encoded())}, extractJSON(&manifest)); err != nil {
				return nil, err
			}
			layersToExtract = append(layersToExtract, layer)
			layersToExtract = append(layersToExtract, manifest.Layers...)
		} else if layer.MediaType == oci.ZarfLayerMediaTypeBlob {
			rel := layer.Annotations[ocispec.AnnotationTitle]
			layersToExtract = append(layersToExtract, layer)
			loaded[rel] = filepath.Join(tp.dst, config.BlobsDir, layer.Digest.Encoded())
		}
	}

	cacheFunc := func(ctx context.Context, file av4.File) error {
		desc := helpers.Find(layersToExtract, func(layer ocispec.Descriptor) bool {
			return layer.Digest.Encoded() == filepath.Base(file.NameInArchive)
		})
		r, err := file.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		return store.Push(ctx, desc, r)
	}

	pathsInArchive := []string{}
	for _, layer := range layersToExtract {
		sha := layer.Digest.Encoded()
		if layer.MediaType == oci.ZarfLayerMediaTypeBlob {
			pathsInArchive = append(pathsInArchive, filepath.Join(config.BlobsDir, sha))
			loaded[sha] = filepath.Join(tp.dst, config.BlobsDir, sha)
		}
	}

	if err := format.Extract(tp.ctx, sourceArchive, pathsInArchive, cacheFunc); err != nil {
		return nil, err
	}

	return loaded, nil
}

// LoadPackage loads a package from a tarball
func (tp *tarballBundleProvider) LoadPackage(sha, destinationDir string, _ int) (PathMap, error) {
	if err := tp.getBundleManifest(); err != nil {
		return nil, err
	}

	format := av4.CompressedArchive{
		Compression: av4.Zstd{},
		Archival:    av4.Tar{},
	}

	sourceArchive, err := os.Open(tp.src)
	if err != nil {
		return nil, err
	}

	var manifest oci.ZarfOCIManifest

	if err := format.Extract(tp.ctx, sourceArchive, []string{filepath.Join(config.BlobsDir, sha)}, extractJSON(&manifest)); err != nil {
		sourceArchive.Close()
		return nil, err
	}

	if err := sourceArchive.Close(); err != nil {
		return nil, err
	}

	extractLayer := func(_ context.Context, file av4.File) error {
		if file.IsDir() {
			return nil
		}
		stream, err := file.Open()
		if err != nil {
			return err
		}
		defer stream.Close()

		desc := helpers.Find(manifest.Layers, func(layer ocispec.Descriptor) bool {
			return layer.Digest.Encoded() == filepath.Base(file.NameInArchive)
		})

		path := desc.Annotations[ocispec.AnnotationTitle]

		size := desc.Size

		dst := filepath.Join(destinationDir, path)

		if err := zarfUtils.CreateDirectory(filepath.Dir(dst), 0700); err != nil {
			return err
		}

		target, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer target.Close()

		written, err := io.Copy(target, stream)
		if err != nil {
			return err
		}
		if written != size {
			return fmt.Errorf("expected to write %d bytes to %s, wrote %d", size, path, written)
		}

		return nil
	}

	layersToExtract := []string{}
	loaded := make(PathMap)

	for _, layer := range manifest.Layers {
		layersToExtract = append(layersToExtract, filepath.Join(config.BlobsDir, layer.Digest.Encoded()))
		loaded[layer.Annotations[ocispec.AnnotationTitle]] = filepath.Join(destinationDir, config.BlobsDir, layer.Digest.Encoded())
	}

	sourceArchive, err = os.Open(tp.src)
	if err != nil {
		return nil, err
	}
	defer sourceArchive.Close()

	if err := format.Extract(tp.ctx, sourceArchive, layersToExtract, extractLayer); err != nil {
		return nil, err
	}

	return loaded, nil
}

// LoadBundleMetadata loads a bundle's metadata from a tarball
func (tp *tarballBundleProvider) LoadBundleMetadata() (PathMap, error) {
	if err := tp.getBundleManifest(); err != nil {
		return nil, err
	}
	pathsToExtract := config.BundleAlwaysPull

	loaded := make(PathMap)

	for _, path := range pathsToExtract {
		layer := tp.manifest.Locate(path)
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
