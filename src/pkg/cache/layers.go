// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cache provides a primitive cache mechanism for bundle layers
package cache

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	ocistore "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
)

// checkLayerExists checks if a layer already exists in the bundle store or the cache, copying it to the dstDir if it does
func CheckLayerExists(ctx context.Context, layer ocispec.Descriptor, store *ocistore.Store, dstDir string) (bool, error) {
	if exists, _ := store.Exists(ctx, layer); exists {
		return true, nil
	} else if Exists(layer.Digest.Encoded()) {
		err := Use(layer.Digest.Encoded(), filepath.Join(dstDir, config.BlobsDir))
		if err == nil {
			return true, nil
		}
	}
	return false, nil
}

// CopyLayers uses ORAS to copy layers from a remote repo to a local OCI store
func CopyLayers(layersToPull []ocispec.Descriptor, estimatedBytes int64, tmpDstDir string, repo *remote.Repository, store *ocistore.Store, artifactName string) (ocispec.Descriptor, error) {
	// copy Zarf pkg
	copyOpts := boci.CreateCopyOpts(layersToPull, config.CommonOptions.OCIConcurrency)
	// Create a thread to update a progress bar as we save the package to disk
	doneSaving := make(chan error)

	// Grab tmpDirSize and add it to the estimatedBytes, otherwise the progress bar will be off
	// because as multiple packages are pulled into the tmpDir, RenderProgressBarForLocalDirWrite continues to
	// add their size which results in strange MB ratios
	tmpDirSize, err := helpers.GetDirSize(tmpDstDir)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	expectedTotalSize := estimatedBytes + tmpDirSize

	go zarfUtils.RenderProgressBarForLocalDirWrite(tmpDstDir, expectedTotalSize, doneSaving, fmt.Sprintf("Pulling: %s", artifactName), fmt.Sprintf("Successfully pulled: %s", artifactName))

	rootDesc, err := oras.Copy(context.TODO(), repo, repo.Reference.String(), store, "", copyOpts)

	doneSaving <- err
	<-doneSaving

	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return rootDesc, nil
}

// AddPulledImgLayers caches the image layers that were just pulled
func AddPulledImgLayers(pulledLayers []ocispec.Descriptor, dstDir string, allLayers bool) (err error) {
	for _, layer := range pulledLayers {
		// when pulling remote 3rd party image layers, they might not have annotations set but we still want to cache them
		// we provide an allLayers flag to cache all layers in this case.
		// TODO: handle potential non-image layers that can get cached when passing allLayers=true
		if allLayers || strings.Contains(layer.Annotations[ocispec.AnnotationTitle], config.BlobsDir) {
			err = Add(filepath.Join(dstDir, config.BlobsDir, layer.Digest.Encoded()))
			if err != nil {
				return err
			}
		}
	}
	return nil
}
