// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/src/config"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Pull pulls a bundle and saves it locally + caches it
func (b *Bundler) Pull() error {
	cacheDir := filepath.Join(zarfConfig.GetAbsCachePath(), "packages")
	// create the cache directory if it doesn't exist
	if err := utils.CreateDirectory(cacheDir, 0755); err != nil {
		return err
	}

	// Get validated source path
	b.cfg.PullOpts.Source = getOciValidatedSource(b.cfg.PullOpts.Source)

	provider, err := NewBundleProvider(context.TODO(), b.cfg.PullOpts.Source, cacheDir)
	if err != nil {
		return err
	}

	// pull the bundle's metadata + sig
	loadedMetadata, err := provider.LoadBundleMetadata()
	if err != nil {
		return err
	}
	if err := utils.ReadYaml(loadedMetadata[config.BundleYAML], &b.bundle); err != nil {
		return err
	}

	// validate the sig (if present)
	if err := ValidateBundleSignature(loadedMetadata[config.BundleYAML], loadedMetadata[config.BundleYAMLSignature], b.cfg.PullOpts.PublicKeyPath); err != nil {
		return err
	}

	// pull the bundle
	loaded, err := provider.LoadBundle(zarfConfig.CommonOptions.OCIConcurrency)
	if err != nil {
		return err
	}

	// create a remote client just to resolve the root descriptor
	remote, err := oci.NewOrasRemote(b.cfg.PullOpts.Source)
	if err != nil {
		return err
	}

	// fetch the bundle's root descriptor
	rootDesc, err := remote.ResolveRoot()
	if err != nil {
		return err
	}

	// make an index.json for this bundle and write to tmp
	index := ocispec.Index{}
	index.SchemaVersion = 2
	ref := fmt.Sprintf("%s-%s", b.bundle.Metadata.Version, b.bundle.Metadata.Architecture)
	annotations := map[string]string{
		ocispec.AnnotationRefName: ref,
	}
	rootDesc.Annotations = annotations // maintain the tag
	index.Manifests = append(index.Manifests, rootDesc)
	bytes, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	indexJSONPath := filepath.Join(b.tmp, "index.json")
	if err := utils.WriteFile(indexJSONPath, bytes); err != nil {
		return err
	}

	// tarball the bundle
	filename := fmt.Sprintf("%s%s-%s-%s.tar.zst", config.BundlePrefix, b.bundle.Metadata.Name, b.bundle.Metadata.Architecture, b.bundle.Metadata.Version)
	dst := filepath.Join(b.cfg.PullOpts.OutputDirectory, filename)

	_ = os.RemoveAll(dst)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	// TODO: support an --uncompressed flag?

	format := archiver.CompressedArchive{
		Compression: archiver.Zstd{},
		Archival:    archiver.Tar{},
	}

	pathMap := make(PathMap)

	// put the index.json and oci-layout at the root of the tarball
	pathMap[filepath.Join(b.tmp, "index.json")] = "index.json"
	pathMap[filepath.Join(cacheDir, "oci-layout")] = "oci-layout"

	// re-map the paths to be relative to the cache directory
	for sha, abs := range loaded {
		if sha == config.BundleYAML || sha == config.BundleYAMLSignature {
			sha = filepath.Base(abs)
		}
		pathMap[abs] = filepath.Join(config.BlobsDir, sha)
	}

	files, err := archiver.FilesFromDisk(nil, pathMap)
	if err != nil {
		return err
	}

	// tarball the bundle
	if err := format.Archive(context.TODO(), out, files); err != nil {
		return err
	}

	message.Debug("Create tarball saved to", dst)

	return nil
}
