// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/mholt/archives"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
)

// Pull pulls a bundle and saves it locally
func (b *Bundle) Pull() error {
	ctx := context.TODO()
	tmpDstDir, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
	if err != nil {
		return fmt.Errorf("pull bundle unable to create temp directory: %w", err)
	}

	// Get validated source path
	source, err := CheckOCISourcePath(b.cfg.PullOpts.Source)
	if err != nil {
		return err
	}
	b.cfg.PullOpts.Source = source

	provider, err := NewBundleProvider(b.cfg.PullOpts.Source, tmpDstDir)
	if err != nil {
		return err
	}

	// pull the bundle's uds-bundle.yaml and it's Zarf pkgs
	bundle, filepaths, err := provider.LoadBundle(b.cfg.PullOpts, zarfConfig.CommonOptions.OCIConcurrency)
	if err != nil {
		return err
	}
	b.bundle = *bundle

	// create a remote client just to resolve the root descriptor
	platform := ocispec.Platform{
		Architecture: config.GetArch(),
		OS:           oci.MultiOS,
	}
	remote, err := zoci.NewRemote(ctx, b.cfg.PullOpts.Source, platform)
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

	pathMap := make(types.PathMap)

	// put the index.json and oci-layout at the root of the tarball
	pathMap[filepath.Join(b.tmp, "index.json")] = "index.json"
	pathMap[filepath.Join(tmpDstDir, "oci-layout")] = "oci-layout"

	// re-map the paths to be relative to the cache directory
	for sha, abs := range filepaths {
		if sha == config.BundleYAML || sha == config.BundleYAMLSignature {
			sha = filepath.Base(abs)
		}
		pathMap[abs] = filepath.Join(config.BlobsDir, sha)
	}

	files, err := archives.FilesFromDisk(context.TODO(), nil, pathMap)
	if err != nil {
		return err
	}

	// tarball the bundle
	if err := config.BundleArchiveFormat.Archive(ctx, out, files); err != nil {
		return err
	}

	message.Debug("Create tarball saved to", dst)

	return nil
}
