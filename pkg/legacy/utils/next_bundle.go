// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package utils

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/mholt/archives"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// CheckLegacyBundlePath rejects local Next bundle definitions and artifacts.
func CheckLegacyBundlePath(source string) error {
	info, err := os.Stat(source)
	if err != nil {
		return nil // Leave missing or inaccessible paths to the operation's validation.
	}
	if info.IsDir() {
		for _, name := range []string{config.BundleYAML, "uds-bundle.yml"} {
			if info, err := os.Stat(filepath.Join(source, name)); err == nil && !info.IsDir() {
				return nil
			}
		}
		for _, name := range []string{"bundle.uds.hcl", "oci/index.json"} {
			if info, err := os.Stat(filepath.Join(source, name)); err == nil && !info.IsDir() {
				return ErrNextBundle
			}
		}
		return nil
	}
	if filepath.Base(source) == "bundle.uds.hcl" {
		return ErrNextBundle
	}
	if !info.Mode().IsRegular() || (!strings.HasSuffix(source, ".tar.zst") && !strings.HasSuffix(source, ".tar")) {
		return nil
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	return config.BundleArchiveFormat.Extract(context.Background(), file, func(_ context.Context, entry archives.FileInfo) error {
		switch path.Clean(entry.NameInArchive) {
		case "oci/index.json":
			return ErrNextBundle
		case "index.json":
			return fs.SkipAll
		}
		return nil
	})
}

// IsNextBundleIndex recognizes both architecture-specific and multi-architecture Next indexes.
func IsNextBundleIndex(index ocispec.Index) bool {
	const bundleType = "application/vnd.defenseunicorns.uds.bundle.v1"
	const definitionType = "application/vnd.defenseunicorns.uds.bundle.definition.v1"
	if index.ArtifactType == bundleType {
		return true
	}
	for _, manifest := range index.Manifests {
		if manifest.ArtifactType == bundleType || manifest.ArtifactType == definitionType {
			return true
		}
	}
	return false
}
