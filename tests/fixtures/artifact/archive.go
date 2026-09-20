// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package artifact provides neutral bundle archive assertions for tests.
package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mholt/archives"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// BundleDefinitionMediaType identifies bundle-definition manifests (ADR-0007).
const BundleDefinitionMediaType = "application/vnd.defenseunicorns.uds.bundle.definition.v1"

const maxSmallEntrySize = 1 << 20

// Entries contains every archive path and the contents of entries small enough
// to contain OCI indexes and manifests.
type Entries struct {
	Paths map[string]bool
	Small map[string][]byte
}

// Read reads the OCI metadata from a bundle archive without retaining large layers.
func Read(ctx context.Context, archivePath string) (entries Entries, err error) {
	entries = Entries{Paths: map[string]bool{}, Small: map[string][]byte{}}
	file, err := os.Open(archivePath)
	if err != nil {
		return Entries{}, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()

	archive := archives.CompressedArchive{Extraction: archives.Tar{}, Compression: archives.Zstd{}}
	err = archive.Extract(ctx, file, func(_ context.Context, info archives.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		entries.Paths[info.NameInArchive] = true
		if info.Size() >= maxSmallEntrySize {
			return nil
		}
		reader, openErr := info.Open()
		if openErr != nil {
			return openErr
		}
		contents, readErr := io.ReadAll(reader)
		if readErr == nil {
			entries.Small[info.NameInArchive] = contents
		}
		return errors.Join(readErr, reader.Close())
	})
	if err != nil {
		return Entries{}, err
	}
	return entries, nil
}

// HasLayer reports whether an OCI manifest has a titled layer whose blob exists.
func (e Entries) HasLayer(title string) (bool, error) {
	return e.hasLayer(title, "")
}

// HasLayerInArtifact reports whether an OCI artifact manifest has a titled layer whose blob exists.
func (e Entries) HasLayerInArtifact(title, artifactType string) (bool, error) {
	return e.hasLayer(title, artifactType)
}

func (e Entries) hasLayer(title, artifactType string) (bool, error) {
	index, err := e.index()
	if err != nil {
		return false, err
	}
	for _, descriptor := range index.Manifests {
		if artifactType != "" && descriptor.ArtifactType != artifactType {
			continue
		}
		manifest, err := e.manifest(descriptor.Digest.String())
		if err != nil {
			return false, err
		}
		for _, layer := range manifest.Layers {
			if layer.Annotations[ocispec.AnnotationTitle] == title {
				return e.Paths[blobPath(layer.Digest.String())], nil
			}
		}
	}
	return false, nil
}

// LayerInArtifact returns a titled layer from a manifest of the given artifact type.
func (e Entries) LayerInArtifact(title, artifactType string) ([]byte, error) {
	index, err := e.index()
	if err != nil {
		return nil, err
	}
	for _, descriptor := range index.Manifests {
		if artifactType != "" && descriptor.ArtifactType != artifactType {
			continue
		}
		manifest, err := e.manifest(descriptor.Digest.String())
		if err != nil {
			return nil, err
		}
		for _, layer := range manifest.Layers {
			if layer.Annotations[ocispec.AnnotationTitle] != title {
				continue
			}
			contents, ok := e.Small[blobPath(layer.Digest.String())]
			if !ok {
				return nil, fmt.Errorf("layer %q blob is not available", title)
			}
			return contents, nil
		}
	}
	return nil, fmt.Errorf("layer %q not found", title)
}

// BlobPaths returns the archive paths for all OCI blob entries.
func (e Entries) BlobPaths() map[string]struct{} {
	paths := map[string]struct{}{}
	for path := range e.Paths {
		if strings.HasPrefix(path, "oci/blobs/sha256/") {
			paths[path] = struct{}{}
		}
	}
	return paths
}

// PackageManifestDigests returns all non-definition manifest digests.
func (e Entries) PackageManifestDigests() (map[string]struct{}, error) {
	index, err := e.index()
	if err != nil {
		return nil, err
	}
	digests := make(map[string]struct{}, len(index.Manifests))
	for _, descriptor := range index.Manifests {
		if descriptor.ArtifactType != BundleDefinitionMediaType {
			digests[descriptor.Digest.String()] = struct{}{}
		}
	}
	return digests, nil
}

func (e Entries) index() (ocispec.Index, error) {
	contents, ok := e.Small["oci/index.json"]
	if !ok {
		return ocispec.Index{}, errors.New("oci/index.json is not available")
	}
	var index ocispec.Index
	if err := json.Unmarshal(contents, &index); err != nil {
		return ocispec.Index{}, err
	}
	return index, nil
}

func (e Entries) manifest(digest string) (ocispec.Manifest, error) {
	contents, ok := e.Small[blobPath(digest)]
	if !ok {
		return ocispec.Manifest{}, fmt.Errorf("manifest %q is not available", digest)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return ocispec.Manifest{}, err
	}
	return manifest, nil
}

func blobPath(digest string) string {
	return "oci/blobs/sha256/" + strings.TrimPrefix(digest, "sha256:")
}
