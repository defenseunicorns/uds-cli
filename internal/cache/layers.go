// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cache stores reusable bundle layers by content digest.
package cache

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/internal/filesystem"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// LayersDirName is the Legacy-compatible directory for cached image layers.
const LayersDirName = "layers"

// WriteLayer streams a SHA-256 layer into the cache and publishes it only after
// its size and digest match desc. The temporary file is created in the layers
// directory so the final rename is atomic even when the cache is on another mount.
func WriteLayer(cacheDir string, desc ocispec.Descriptor, src io.Reader) error {
	if cacheDir == "" {
		return errors.New("cache directory is required")
	}
	if err := desc.Digest.Validate(); err != nil {
		return fmt.Errorf("invalid layer digest %q: %w", desc.Digest, err)
	}
	if desc.Digest.Algorithm() != godigest.SHA256 {
		return fmt.Errorf("unsupported cache layer digest algorithm %q", desc.Digest.Algorithm())
	}
	if desc.Size < 0 {
		return fmt.Errorf("invalid layer size %d", desc.Size)
	}

	layersDir := filepath.Join(cacheDir, LayersDirName)
	if err := os.MkdirAll(layersDir, filesystem.PrivateDirectoryMode); err != nil {
		return fmt.Errorf("creating cache layers directory %q: %w", layersDir, err)
	}
	tmp, err := os.CreateTemp(layersDir, ".uds-layer-*")
	if err != nil {
		return fmt.Errorf("creating temporary cache layer in %q: %w", layersDir, err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	digester := godigest.SHA256.Digester()
	size, err := io.Copy(io.MultiWriter(tmp, digester.Hash()), src)
	if err != nil {
		return fmt.Errorf("writing cache layer %s: %w", desc.Digest, err)
	}
	if size != desc.Size || digester.Digest() != desc.Digest {
		return fmt.Errorf("cache layer %s content does not match its descriptor", desc.Digest)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary cache layer %s: %w", desc.Digest, err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(layersDir, desc.Digest.Encoded())); err != nil {
		return fmt.Errorf("publishing cache layer %s: %w", desc.Digest, err)
	}
	return nil
}
