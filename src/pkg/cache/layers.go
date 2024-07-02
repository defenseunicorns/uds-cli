// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cache provides a primitive cache mechanism for bundle layers
package cache

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	ocistore "oras.land/oras-go/v2/content/oci"
)

// CheckLayerExists checks if a layer already exists in the bundle store or the cache, copying it to the dstDir if it does
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

// AddPulledImgLayers caches the image layers that were just pulled
func AddPulledImgLayers(pulledLayers []ocispec.Descriptor, dstDir string) (err error) {
	for _, layer := range pulledLayers {
		// layers with blobs/sha256 in their title are image layers, as shown in the Zarf image manifest
		if strings.Contains(layer.Annotations[ocispec.AnnotationTitle], config.BlobsDir) {
			err = Add(filepath.Join(dstDir, config.BlobsDir, layer.Digest.Encoded()))
			if err != nil {
				return err
			}
		}
	}
	return nil
}
