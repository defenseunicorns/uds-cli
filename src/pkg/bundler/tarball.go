// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundler defines behavior for bundling packages
package bundler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	av3 "github.com/mholt/archiver/v3"
	av4 "github.com/mholt/archiver/v4"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"io"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/file"
	ocistore "oras.land/oras-go/v2/content/oci"
	"os"
	"path/filepath"
)

// LocalBundler contains methods for loading local Zarf packages into a bundle
type LocalBundler struct {
	ctx          context.Context
	tarballSrc   string
	extractedDst string
}

// NewLocalBundler creates a bundler for bundling local Zarf pkgs
func NewLocalBundler(src, dest string) LocalBundler {
	return LocalBundler{tarballSrc: src, extractedDst: dest, ctx: context.TODO()}
}

// GetMetadata grabs metadata from a local Zarf package's zarf.yaml
func (b *LocalBundler) GetMetadata(pathToTarball string, tmpDir string) (zarfTypes.ZarfPackage, error) {
	zarfTarball, err := os.Open(pathToTarball)
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	format := av4.CompressedArchive{
		Compression: av4.Zstd{},
		Archival:    av4.Tar{},
	}
	if err := format.Extract(context.TODO(), zarfTarball, []string{config.ZarfYAML}, func(_ context.Context, fileInArchive av4.File) error {
		// write zarf.yaml to tmp for checking optional components later on
		dst := filepath.Join(tmpDir, fileInArchive.NameInArchive)
		outFile, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer outFile.Close()
		stream, err := fileInArchive.Open()
		if err != nil {
			return err
		}
		defer stream.Close()
		_, err = io.Copy(outFile, io.Reader(stream))
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		zarfTarball.Close()
		return zarfTypes.ZarfPackage{}, err
	}
	zarfYAML := zarfTypes.ZarfPackage{}
	zarfYAMLPath := filepath.Join(tmpDir, config.ZarfYAML)
	err = utils.ReadYaml(zarfYAMLPath, &zarfYAML)
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	return zarfYAML, err
}

// Extract extracts a compressed Zarf archive into a directory
func (b *LocalBundler) Extract() error {
	err := av3.Unarchive(b.tarballSrc, b.extractedDst) // todo: awkward to use old version of mholt/archiver
	if err != nil {
		return err
	}
	return nil
}

// Load loads a zarf.yaml into a Zarf object
func (b *LocalBundler) Load() (zarfTypes.ZarfPackage, error) {
	// grab zarf.yaml from extracted archive
	p, err := os.ReadFile(filepath.Join(b.extractedDst, config.ZarfYAML))
	if err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	var pkg zarfTypes.ZarfPackage
	if err := goyaml.Unmarshal(p, &pkg); err != nil {
		return zarfTypes.ZarfPackage{}, err
	}
	return pkg, err
}

// ToBundle transfers a Zarf package to a given Bundle
func (b *LocalBundler) ToBundle(bundleStore *ocistore.Store, pkg zarfTypes.ZarfPackage, artifactPathMap map[string]string, bundleTmpDir string, packageTmpDir string) (ocispec.Descriptor, error) {
	// todo: only grab components that are required + specified in optional-components
	ctx := b.ctx
	src, err := file.New(packageTmpDir)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	// Grab Zarf layers
	paths := []string{}
	err = filepath.Walk(packageTmpDir, func(path string, info os.FileInfo, err error) error {
		// Catch any errors that happened during the walk
		if err != nil {
			return err
		}

		// Add any resource that is not a directory to the paths of objects we will include into the package
		if !info.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("unable to get the layers in the package to publish: %w", err)
	}

	var descs []ocispec.Descriptor
	for _, path := range paths {
		name, err := filepath.Rel(packageTmpDir, path)
		if err != nil {
			return ocispec.Descriptor{}, err
		}

		mediaType := oci.ZarfLayerMediaTypeBlob

		// get descriptor, push bytes
		desc, err := src.Add(ctx, name, mediaType, path)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
		layer, err := src.Fetch(ctx, desc)
		if err != nil {
			return ocispec.Descriptor{}, err
		}

		// push if layer doesn't already exist in bundleStore
		if exists, err := bundleStore.Exists(ctx, desc); !exists && err == nil {
			if err := bundleStore.Push(ctx, desc, layer); err != nil {
				return ocispec.Descriptor{}, err
			}
		} else if err != nil {
			return ocispec.Descriptor{}, err
		}

		digest := desc.Digest.Encoded()
		artifactPathMap[filepath.Join(bundleTmpDir, config.BlobsDir, digest)] = filepath.Join(config.BlobsDir, digest)
		descs = append(descs, desc)
	}
	// push the manifest config
	manifestConfigDesc, err := pushZarfManifestConfigFromMetadata(bundleStore, &pkg.Metadata, &pkg.Build)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	// push the manifest
	rootManifest, err := generatePkgManifest(bundleStore, descs, manifestConfigDesc)

	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return rootManifest, err
}

// todo: clean up code the following which comes from Zarf
func pushZarfManifestConfigFromMetadata(store *ocistore.Store, metadata *zarfTypes.ZarfMetadata, build *zarfTypes.ZarfBuildData) (ocispec.Descriptor, error) {
	ctx := context.TODO()
	annotations := map[string]string{
		ocispec.AnnotationTitle:       metadata.Name,
		ocispec.AnnotationDescription: metadata.Description,
	}
	manifestConfig := oci.ConfigPartial{
		Architecture: build.Architecture,
		OCIVersion:   "1.0.1",
		Annotations:  annotations,
	}
	manifestConfigBytes, err := json.Marshal(manifestConfig)
	manifestConfigDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageConfig, manifestConfigBytes)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	if err := store.Push(ctx, manifestConfigDesc, bytes.NewReader(manifestConfigBytes)); err != nil {
		return ocispec.Descriptor{}, err
	}

	return manifestConfigDesc, err
}

func generatePkgManifest(store *ocistore.Store, descs []ocispec.Descriptor, configDesc ocispec.Descriptor) (ocispec.Descriptor, error) {
	ctx := context.TODO()

	// adopted from oras.Pack fn
	// manually  build the manifest and push to store and save reference
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2, // historical value. does not pertain to OCI or docker version
		},
		Config:    configDesc,
		MediaType: ocispec.MediaTypeImageManifest,
		Layers:    descs,
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to marshal manifest: %w", err)
	}
	manifestDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestJSON)

	// push manifest
	if err := store.Push(ctx, manifestDesc, bytes.NewReader(manifestJSON)); err != nil {
		return ocispec.Descriptor{}, err
	}
	return manifestDesc, nil
}
