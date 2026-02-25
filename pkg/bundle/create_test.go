// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mholt/archives"
	"github.com/stretchr/testify/require"
)

func TestCreate_BuildsTarZstWithExpectedLayout(t *testing.T) {
	dir := t.TempDir()

	localLayout := filepath.Join(dir, "localpkg")
	digests := writeMinimalOCILayout(t, localLayout)
	manifestHex, configHex, layerHex := digests.ManifestHex, digests.ConfigHex, digests.LayerHex

	valuesDir := filepath.Join(dir, "values")
	require.NoError(t, os.MkdirAll(valuesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "a.yaml"), []byte("a: 1\n"), 0o644))

	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "test"
  version = "0.0.1"
}

package "pkg1" {
  source = "localpkg"
  values_files = ["values/a.yaml"]
}
`), 0o644))

	_, err := Create(context.Background(), CreateOptions{BundleFile: bundleFile, Out: io.Discard})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-test-"+runtime.GOARCH+"-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	require.Contains(t, entries, "uds-bundle.hcl")
	require.Contains(t, entries, "values/pkg1/0.yaml")
	require.Equal(t, "a: 1\n", string(entries["values/pkg1/0.yaml"]))

	require.Contains(t, entries, "oci/oci-layout")
	require.Contains(t, entries, "oci/index.json")
	require.Contains(t, entries, "oci/blobs/sha256/"+manifestHex)
	require.Contains(t, entries, "oci/blobs/sha256/"+configHex)
	require.Contains(t, entries, "oci/blobs/sha256/"+layerHex)
}

func TestSanitizeFileComponent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"simple", "simple"},
		{"hello-world", "hello-world"},
		{"v1.2.3", "v1.2.3"},
		{"my/bundle", "my-bundle"},
		{"with spaces", "with-spaces"},
		{"UPPER", "UPPER"},
		{"--leading-dashes--", "leading-dashes"},
		{"a/b/c", "a-b-c"},
		{"!@#$%", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.want, sanitizeFileComponent(tt.input))
		})
	}
}

func TestBundleOutputName(t *testing.T) {
	tests := []struct {
		name    string
		bundle  UDSBundle
		wantSuf string
	}{
		{
			name:    "full name arch version",
			bundle:  UDSBundle{Metadata: Metadata{Name: "my-bundle", Version: "1.0.0"}},
			wantSuf: "uds-bundle-my-bundle-" + runtime.GOARCH + "-1.0.0.tar.zst",
		},
		{
			name:    "no version",
			bundle:  UDSBundle{Metadata: Metadata{Name: "my-bundle"}},
			wantSuf: "uds-bundle-my-bundle-" + runtime.GOARCH + ".tar.zst",
		},
		{
			name:    "name with special chars",
			bundle:  UDSBundle{Metadata: Metadata{Name: "my/bundle", Version: "1.0.0"}},
			wantSuf: "uds-bundle-my-bundle-" + runtime.GOARCH + "-1.0.0.tar.zst",
		},
		{
			name:    "empty name defaults to bundle",
			bundle:  UDSBundle{Metadata: Metadata{Name: "", Version: "1.0.0"}},
			wantSuf: "uds-bundle-bundle-" + runtime.GOARCH + "-1.0.0.tar.zst",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantSuf, bundleOutputName(&tt.bundle, runtime.GOARCH))
		})
	}
}

// TestCreate_MultiArchLocalLayout verifies that when a local package directory
// contains a multi-platform OCI index, only the manifest for the requested
// architecture is ingested into the bundle.
func TestCreate_MultiArchLocalLayout(t *testing.T) {
	dir := t.TempDir()

	// Create a local two-arch OCI layout (amd64 + arm64).
	localLayout := filepath.Join(dir, "multipkg")
	writeMultiArchOCILayout(t, localLayout, []string{"amd64", "arm64"})

	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "multiarch-test"
  version = "0.0.1"
}

package "pkg1" {
  source = "multipkg"
}
`), 0o644))

	// Build for amd64 explicitly.
	_, err := Create(context.Background(), CreateOptions{
		BundleFile: bundleFile,
		Arch:       "amd64",
		Out:        io.Discard,
	})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-multiarch-test-amd64-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	// Parse the bundle's OCI index and collect manifest digests.
	idxRaw, ok := entries["oci/index.json"]
	require.True(t, ok, "oci/index.json not found in bundle")

	type indexEntry struct {
		Digest   string `json:"digest"`
		Platform *struct {
			Architecture string `json:"architecture"`
		} `json:"platform,omitempty"`
	}
	type indexFile struct {
		Manifests []indexEntry `json:"manifests"`
	}
	var idx indexFile
	require.NoError(t, json.Unmarshal(idxRaw, &idx))

	// Exactly one manifest should be present and it should be amd64.
	require.Len(t, idx.Manifests, 1, "expected exactly one platform manifest in bundle index")
	require.NotNil(t, idx.Manifests[0].Platform)
	require.Equal(t, "amd64", idx.Manifests[0].Platform.Architecture)
}

// TestCreate_OptionalComponentBlobsRemoved verifies that when a Zarf package is
// ingested without listing any optional_components, the optional component layer
// blobs are absent from the output bundle (not just missing from the manifest).
func TestCreate_OptionalComponentBlobsRemoved(t *testing.T) {
	dir := t.TempDir()

	// Create a Zarf-like local layout: "core" (required) + "logging" (optional).
	layoutDir := filepath.Join(dir, "zarfpkg")
	compDigests := writeZarfLikeOCILayout(t, layoutDir, []string{"core"}, []string{"logging"})

	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "gc-test"
  version = "0.0.1"
}

package "pkg1" {
  source = "zarfpkg"
}
`), 0o644))

	_, err := Create(context.Background(), CreateOptions{
		BundleFile: bundleFile,
		Arch:       runtime.GOARCH,
		Out:        io.Discard,
	})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-gc-test-"+runtime.GOARCH+"-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	coreBlob := "oci/blobs/sha256/" + compDigests["core"]
	require.Contains(t, entries, coreBlob, "required component blob should be present")

	loggingBlob := "oci/blobs/sha256/" + compDigests["logging"]
	require.NotContains(t, entries, loggingBlob, "optional component blob should be absent when not requested")
}

func readTarZstEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()

	f, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	ca := archives.CompressedArchive{
		Extraction:  archives.Tar{},
		Compression: archives.Zstd{},
	}
	entries := map[string][]byte{}
	err = ca.Extract(context.Background(), f, func(_ context.Context, info archives.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		rc, err := info.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			return err
		}
		entries[info.NameInArchive] = b
		return nil
	})
	require.NoError(t, err)
	return entries
}
