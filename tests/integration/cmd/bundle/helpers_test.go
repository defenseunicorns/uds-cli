// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package bundle_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mholt/archives"
	"github.com/stretchr/testify/require"
)

// readBundleEntries reads a bundle tar.zst and returns:
//   - allPaths: set of every file path in the archive
//   - small: content of files smaller than 1 MiB (suitable for OCI index / manifest blobs)
//
// Large blobs (layer data) are tracked in allPaths only, avoiding loading multi-hundred-MB
// layer files into memory during tests.
func readBundleEntries(t *testing.T, tarPath string) (allPaths map[string]bool, small map[string][]byte) {
	t.Helper()
	allPaths = map[string]bool{}
	small = map[string][]byte{}

	f, err := os.Open(tarPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	ca := archives.CompressedArchive{
		Extraction:  archives.Tar{},
		Compression: archives.Zstd{},
	}
	err = ca.Extract(t.Context(), f, func(_ context.Context, info archives.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		allPaths[info.NameInArchive] = true
		const maxSmall = 1 << 20 // 1 MiB
		if info.Size() < maxSmall {
			rc, openErr := info.Open()
			if openErr != nil {
				return openErr
			}
			defer rc.Close()
			b, readErr := io.ReadAll(rc)
			if readErr != nil {
				return readErr
			}
			small[info.NameInArchive] = b
		}
		return nil
	})
	require.NoError(t, err)
	return allPaths, small
}

// bundleContainsLayerTitle reports whether the bundle archive at tarPath contains:
//  1. a layer entry in a manifest whose org.opencontainers.image.title == title, AND
//  2. the corresponding blob file is present in the archive.
//
// It searches all manifests listed in oci/index.json.
func bundleContainsLayerTitle(t *testing.T, tarPath, title string) bool {
	t.Helper()
	allPaths, small := readBundleEntries(t, tarPath)

	idxBytes, ok := small["oci/index.json"]
	require.True(t, ok, "oci/index.json not found in bundle")

	var idx struct {
		Manifests []struct {
			Digest string `json:"digest"`
		} `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal(idxBytes, &idx))

	for _, m := range idx.Manifests {
		hex := strings.TrimPrefix(m.Digest, "sha256:")
		manifestBytes, hasManifest := small["oci/blobs/sha256/"+hex]
		if !hasManifest {
			continue
		}
		var im struct {
			Layers []struct {
				Digest      string            `json:"digest"`
				Annotations map[string]string `json:"annotations"`
			} `json:"layers"`
		}
		if err := json.Unmarshal(manifestBytes, &im); err != nil {
			continue
		}
		for _, l := range im.Layers {
			if l.Annotations["org.opencontainers.image.title"] == title {
				layerHex := strings.TrimPrefix(l.Digest, "sha256:")
				return allPaths["oci/blobs/sha256/"+layerHex]
			}
		}
	}
	return false
}

// testDataPath returns the absolute path to a file under tests/test_data/.
func testDataPath(t *testing.T, relPath string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	// thisFile is tests/integration/cmd/bundle/helpers_test.go
	// project root is 4 directories up
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	return filepath.Join(root, "tests", "test_data", relPath)
}
