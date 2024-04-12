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

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/zoci"
	"github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Pull pulls a bundle and saves it locally
func (b *Bundle) Pull() error {
	ctx := context.TODO()
	// use uds-cache/packages as the dst dir for the pull to get auto caching
	// we use an ORAS ocistore to make that dir look like an OCI artifact
	cacheDir := filepath.Join(zarfConfig.GetAbsCachePath(), "packages")
	if err := helpers.CreateDirectory(cacheDir, 0o755); err != nil {
		return err
	}

	// Get validated source path
	source, err := CheckOCISourcePath(b.cfg.PullOpts.Source)
	if err != nil {
		return err
	}
	b.cfg.PullOpts.Source = source

	provider, err := NewBundleProvider(b.cfg.PullOpts.Source, cacheDir)
	if err != nil {
		return err
	}

	// pull the bundle's uds-bundle.yaml and it's Zarf pkgs
	bundle, loaded, err := provider.LoadBundle(b.cfg.PullOpts, zarfConfig.CommonOptions.OCIConcurrency)
	if err != nil {
		return err
	}
	b.bundle = *bundle

	// create a remote client just to resolve the root descriptor
	platform := ocispec.Platform{
		Architecture: config.GetArch(),
		OS:           oci.MultiOS,
	}
	remote, err := zoci.NewRemote(b.cfg.PullOpts.Source, platform)
	if err != nil {
		return err
	}

	// fetch the bundle's root descriptor
	rootDesc, err := remote.ResolveRoot(ctx)
	if err != nil {
		return err
	}

	// make an index.json for this bundle and write to tmp
	index := ocispec.Index{}
	index.SchemaVersion = 2
	ref := b.bundle.Metadata.Version
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
	if err := os.WriteFile(indexJSONPath, bytes, helpers.ReadWriteUser); err != nil {
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

	pathMap := make(types.PathMap)

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
	if err := format.Archive(ctx, out, files); err != nil {
		return err
	}

	message.Debug("Create tarball saved to", dst)

	return nil
}
