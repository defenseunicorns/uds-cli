// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package bundle_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/mholt/archives"
	"github.com/stretchr/testify/require"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func createInspectArtifact(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	pkgDir := filepath.Join(root, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "zarf.yaml"), []byte("build:\n  signed: true\nmetadata:\n  name: test\n  version: 1.0.0\n"), 0o644))

	bundleFile := filepath.Join(root, bundlepkg.BundleFileName)
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "inspect-integration"
  version = "1.0.0"
}
package "pkg" {
  source = "pkg"
  signature_verification { verify = false }
}
`), 0o644))

	defaults := bundlepkg.ConfigOptions{
		Architecture: runtime.GOARCH,
		Concurrency:  10,
		LogLevel:     "info",
		TmpDir:       t.TempDir(),
	}
	result, err := bundlepkg.Create(t.Context(), bundlepkg.CreateOptions{
		Config:     &bundlepkg.UDSBundleConfig{Global: &bundlepkg.GlobalOptions{LogLevel: "info"}, Options: &defaults},
		BundleFile: bundleFile,
		Streams:    iostreams.IOStreams{},
	})
	require.NoError(t, err)
	return result.OutputPath
}

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

// bundleContainsLayerTitle reports whether the given bundle entries contain:
//  1. a layer in any manifest whose org.opencontainers.image.title == title, AND
//  2. the corresponding blob present in allPaths.
func bundleContainsLayerTitle(t *testing.T, allPaths map[string]bool, small map[string][]byte, title string) bool {
	t.Helper()

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

// bundleDefinitionContainsLayerTitle reports whether the bundle definition manifest
// (identified by artifactType == MediaTypeBundleDefinition) contains a layer with
// the given org.opencontainers.image.title AND the corresponding blob is present.
func bundleDefinitionContainsLayerTitle(t *testing.T, allPaths map[string]bool, small map[string][]byte, title string) bool {
	t.Helper()

	idxBytes, ok := small["oci/index.json"]
	require.True(t, ok, "oci/index.json not found in bundle")

	var idx struct {
		Manifests []struct {
			Digest       string `json:"digest"`
			ArtifactType string `json:"artifactType"`
		} `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal(idxBytes, &idx))

	for _, m := range idx.Manifests {
		if m.ArtifactType != bundlepkg.MediaTypeBundleDefinition {
			continue
		}
		hex := strings.TrimPrefix(m.Digest, "sha256:")
		manifestBytes, ok := small["oci/blobs/sha256/"+hex]
		if !ok {
			continue
		}
		var im struct {
			Layers []struct {
				Digest      string            `json:"digest"`
				Annotations map[string]string `json:"annotations"`
			} `json:"layers"`
		}
		require.NoError(t, json.Unmarshal(manifestBytes, &im))
		for _, l := range im.Layers {
			if l.Annotations["org.opencontainers.image.title"] == title {
				layerHex := strings.TrimPrefix(l.Digest, "sha256:")
				return allPaths["oci/blobs/sha256/"+layerHex]
			}
		}
	}
	return false
}

// startLocalRegistry starts an in-memory OCI registry and returns its host (host:port).
func startLocalRegistry(t *testing.T) string {
	t.Helper()
	s := httptest.NewServer(registry.New())
	t.Cleanup(s.Close)
	return strings.TrimPrefix(s.URL, "http://")
}

// startLocalTLSRegistry starts an in-memory OCI registry with a self-signed TLS certificate.
func startLocalTLSRegistry(t *testing.T) string {
	t.Helper()
	s := httptest.NewTLSServer(registry.New())
	t.Cleanup(s.Close)
	return strings.TrimPrefix(s.URL, "https://")
}

// extractLayerFromBundle extracts a layer's blob content by title from the bundle definition manifest.
func extractLayerFromBundle(t *testing.T, small map[string][]byte, title string) []byte {
	t.Helper()

	idxBytes, ok := small["oci/index.json"]
	require.True(t, ok)

	var idx struct {
		Manifests []struct {
			Digest       string `json:"digest"`
			ArtifactType string `json:"artifactType"`
		} `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal(idxBytes, &idx))

	for _, m := range idx.Manifests {
		if m.ArtifactType != bundlepkg.MediaTypeBundleDefinition {
			continue
		}
		hex := strings.TrimPrefix(m.Digest, "sha256:")
		manifestBytes, ok := small["oci/blobs/sha256/"+hex]
		if !ok {
			continue
		}
		var manifest struct {
			Layers []struct {
				Digest      string            `json:"digest"`
				Annotations map[string]string `json:"annotations"`
			} `json:"layers"`
		}
		require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
		for _, l := range manifest.Layers {
			if l.Annotations["org.opencontainers.image.title"] == title {
				layerHex := strings.TrimPrefix(l.Digest, "sha256:")
				data, ok := small["oci/blobs/sha256/"+layerHex]
				require.True(t, ok, "%s blob not found", title)
				return data
			}
		}
	}
	t.Fatalf("%s layer not found in bundle definition manifest", title)
	return nil
}

// assertHasReconfiguredAnnotation verifies the bundle definition manifest has the provenance annotation.
func assertHasReconfiguredAnnotation(t *testing.T, small map[string][]byte) string {
	t.Helper()
	value := reconfiguredAnnotation(t, small)
	if !strings.HasPrefix(value, "sha256:") {
		t.Fatal("reconfigured-from annotation should be a sha256 digest")
	}
	return value
}

func reconfiguredAnnotation(t *testing.T, small map[string][]byte) string {
	t.Helper()

	idxBytes, ok := small["oci/index.json"]
	require.True(t, ok)

	var idx struct {
		Manifests []struct {
			Digest       string `json:"digest"`
			ArtifactType string `json:"artifactType"`
		} `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal(idxBytes, &idx))

	for _, m := range idx.Manifests {
		if m.ArtifactType != bundlepkg.MediaTypeBundleDefinition {
			continue
		}
		hex := strings.TrimPrefix(m.Digest, "sha256:")
		manifestBytes, ok := small["oci/blobs/sha256/"+hex]
		require.True(t, ok)

		var manifest struct {
			Annotations map[string]string `json:"annotations"`
		}
		require.NoError(t, json.Unmarshal(manifestBytes, &manifest))

		value, has := manifest.Annotations[bundlepkg.AnnotationReconfiguredFrom]
		if !has {
			t.Fatal("bundle definition manifest missing reconfigured-from annotation")
		}
		return value
	}
	t.Fatal("bundle definition manifest not found")
	return ""
}
