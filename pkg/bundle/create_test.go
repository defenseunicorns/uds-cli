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
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/mholt/archives"
	"github.com/stretchr/testify/require"
)

// newTestConfig returns a *UDSBundleConfig with defaults suitable for tests.
func newTestConfig() *UDSBundleConfig {
	opts := ConfigOptions{
		Architecture: runtime.GOARCH,
		TmpDir:       os.TempDir(),
		Concurrency:  10,
	}
	return &UDSBundleConfig{Global: &GlobalOptions{}, Options: &opts}
}

// newTestConfigWithArch returns a *UDSBundleConfig with the given architecture.
func newTestConfigWithArch(arch string) *UDSBundleConfig {
	opts := ConfigOptions{
		Architecture: arch,
		TmpDir:       os.TempDir(),
		Concurrency:  10,
	}
	return &UDSBundleConfig{Global: &GlobalOptions{}, Options: &opts}
}

func TestCreate_BuildsTarZstWithExpectedLayout(t *testing.T) {
	dir := t.TempDir()

	localLayout := filepath.Join(dir, "localpkg")
	digests := writeMinimalOCILayout(t, localLayout)
	manifestHex, configHex, layerHex := digests.ManifestHex, digests.ConfigHex, digests.LayerHex

	valuesDir := filepath.Join(dir, "values")
	require.NoError(t, os.MkdirAll(valuesDir, tempDirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "a.yaml"), []byte("a: 1\n"), tmpFilePerm))

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
`), tmpFilePerm))

	_, err := Create(t.Context(), CreateOptions{
		Config:     newTestConfig(),
		BundleFile: bundleFile,
		Streams:    iostreams.New(nil, nil, io.Discard),
	})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-test-"+runtime.GOARCH+"-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	require.Contains(t, entries, "oci/oci-layout")
	require.Contains(t, entries, "oci/index.json")
	require.Contains(t, entries, "oci/blobs/sha256/"+manifestHex)
	require.Contains(t, entries, "oci/blobs/sha256/"+configHex)
	require.Contains(t, entries, "oci/blobs/sha256/"+layerHex)
	require.True(t, bundleDefinitionContainsLayerTitle(t, entries, "bundle.uds.hcl"))
	require.True(t, bundleDefinitionContainsLayerTitle(t, entries, "values/pkg1/0.yaml"))

	// Parse the OCI index and locate the bundle definition manifest by artifactType.
	var idx struct {
		Manifests []struct {
			Digest       string `json:"digest"`
			ArtifactType string `json:"artifactType"`
		} `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))
	require.Len(t, idx.Manifests, 2) // config manifest + package manifest

	var cfgDigest string
	for _, m := range idx.Manifests {
		if m.ArtifactType == MediaTypeBundleDefinition {
			cfgDigest = m.Digest
		}
	}
	require.NotEmpty(t, cfgDigest, "bundle definition manifest not found in OCI index")

	// Parse the config manifest and verify layer titles and content.
	cfgBlob := entries["oci/blobs/sha256/"+strings.TrimPrefix(cfgDigest, "sha256:")]
	require.NotNil(t, cfgBlob)

	var cfgManifest struct {
		ArtifactType string `json:"artifactType"`
		Layers       []struct {
			Digest      string            `json:"digest"`
			Annotations map[string]string `json:"annotations"`
		} `json:"layers"`
	}
	require.NoError(t, json.Unmarshal(cfgBlob, &cfgManifest))
	require.Equal(t, MediaTypeBundleDefinition, cfgManifest.ArtifactType)
	require.Len(t, cfgManifest.Layers, 2) // HCL + 1 values file

	titles := make([]string, len(cfgManifest.Layers))
	for i, l := range cfgManifest.Layers {
		titles[i] = l.Annotations["org.opencontainers.image.title"]
	}
	require.Contains(t, titles, "bundle.uds.hcl")
	require.Contains(t, titles, "values/pkg1/0.yaml")

	// Verify the values file content is preserved in the blob.
	for _, l := range cfgManifest.Layers {
		if l.Annotations["org.opencontainers.image.title"] == "values/pkg1/0.yaml" {
			blob := entries["oci/blobs/sha256/"+strings.TrimPrefix(l.Digest, "sha256:")]
			require.Equal(t, "a: 1\n", string(blob))
		}
	}
}

func TestCreate_SharedValuesFileDeduplicatedInOCIStore(t *testing.T) {
	dir := t.TempDir()

	writeMinimalOCILayout(t, filepath.Join(dir, "pkg1"))
	writeMinimalOCILayout(t, filepath.Join(dir, "pkg2"))

	valuesDir := filepath.Join(dir, "values")
	require.NoError(t, os.MkdirAll(valuesDir, tempDirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "shared.yaml"), []byte("shared: true\n"), tmpFilePerm))

	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "shared-values-test"
  version = "0.0.1"
}

package "pkg1" {
  source = "pkg1"
  values_files = ["values/shared.yaml"]
}

package "pkg2" {
  source = "pkg2"
  values_files = ["values/shared.yaml"]
}
`), tmpFilePerm))

	_, err := Create(t.Context(), CreateOptions{
		Config:     newTestConfig(),
		BundleFile: bundleFile,
		Streams:    iostreams.New(nil, nil, io.Discard),
	})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-shared-values-test-"+runtime.GOARCH+"-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	// Locate the bundle definition manifest.
	var idx struct {
		Manifests []struct {
			Digest       string `json:"digest"`
			ArtifactType string `json:"artifactType"`
		} `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))

	var defManifestBytes []byte
	for _, m := range idx.Manifests {
		if m.ArtifactType == MediaTypeBundleDefinition {
			hex := strings.TrimPrefix(m.Digest, "sha256:")
			defManifestBytes = entries["oci/blobs/sha256/"+hex]
			break
		}
	}
	require.NotNil(t, defManifestBytes, "bundle definition manifest not found")

	var defManifest struct {
		Layers []struct {
			Digest      string            `json:"digest"`
			Annotations map[string]string `json:"annotations"`
		} `json:"layers"`
	}
	require.NoError(t, json.Unmarshal(defManifestBytes, &defManifest))

	// Each package must have its own titled layer entry.
	layersByTitle := make(map[string]string)
	for _, l := range defManifest.Layers {
		title := l.Annotations["org.opencontainers.image.title"]
		layersByTitle[title] = l.Digest
	}
	require.Contains(t, layersByTitle, "values/pkg1/0.yaml")
	require.Contains(t, layersByTitle, "values/pkg2/0.yaml")

	// Both layer entries must point to the same blob digest.
	require.Equal(t, layersByTitle["values/pkg1/0.yaml"], layersByTitle["values/pkg2/0.yaml"],
		"shared values file content should map to a single deduplicated blob")

	// Only one blob should exist in the OCI store for that digest.
	blobPath := "oci/blobs/sha256/" + strings.TrimPrefix(layersByTitle["values/pkg1/0.yaml"], "sha256:")
	require.Contains(t, entries, blobPath)
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
`), tmpFilePerm))

	// Build for amd64 explicitly.
	_, err := Create(t.Context(), CreateOptions{
		Config:     newTestConfigWithArch("amd64"),
		BundleFile: bundleFile,
		Streams:    iostreams.New(nil, nil, io.Discard),
	})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-multiarch-test-amd64-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	// Parse the bundle's OCI index and collect manifest digests.
	idxRaw, ok := entries["oci/index.json"]
	require.True(t, ok, "oci/index.json not found in bundle")

	type indexEntry struct {
		Digest       string `json:"digest"`
		ArtifactType string `json:"artifactType,omitempty"`
		Platform     *struct {
			Architecture string `json:"architecture"`
		} `json:"platform,omitempty"`
	}
	type indexFile struct {
		Manifests []indexEntry `json:"manifests"`
	}
	var idx indexFile
	require.NoError(t, json.Unmarshal(idxRaw, &idx))

	// Filter out the bundle definition manifest; only package manifests remain.
	var pkgManifests []indexEntry
	for _, m := range idx.Manifests {
		if m.ArtifactType != MediaTypeBundleDefinition {
			pkgManifests = append(pkgManifests, m)
		}
	}

	// Exactly one package manifest should be present and it should be amd64.
	require.Len(t, pkgManifests, 1, "expected exactly one platform manifest in bundle index")
	require.NotNil(t, pkgManifests[0].Platform)
	require.Equal(t, "amd64", pkgManifests[0].Platform.Architecture)
}

// TestCreate_OptionalComponentBlobsRemoved verifies that when a Zarf package is
// ingested without listing any optional_components, the optional component layer
// blobs are absent from the output bundle (not just missing from the manifest).
func TestCreate_OptionalComponentBlobsRemoved(t *testing.T) {
	dir := t.TempDir()

	// Create a Zarf-like local layout: "core" (required) + "logging" (optional).
	layoutDir := filepath.Join(dir, "zarfpkg")
	digests := WriteZarfLikeOCILayout(t, layoutDir, []string{"core"}, []string{"logging"})
	compDigests := digests.ComponentHexes

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
`), tmpFilePerm))

	_, err := Create(t.Context(), CreateOptions{
		Config:     newTestConfig(),
		BundleFile: bundleFile,
		Streams:    iostreams.New(nil, nil, io.Discard),
	})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-gc-test-"+runtime.GOARCH+"-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	coreBlob := "oci/blobs/sha256/" + compDigests["core"]
	require.Contains(t, entries, coreBlob, "required component blob should be present")

	loggingBlob := "oci/blobs/sha256/" + compDigests["logging"]
	require.NotContains(t, entries, loggingBlob, "optional component blob should be absent when not requested")
}

func TestCreate_DefaultsHCLIncludedWhenPresent(t *testing.T) {
	dir := t.TempDir()

	writeMinimalOCILayout(t, filepath.Join(dir, "localpkg"))

	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "defaults-test"
  version = "0.0.1"
}

package "pkg1" {
  source = "localpkg"
}
`), tmpFilePerm))

	defaultsContent := []byte(`options {
  architecture = "amd64"
}

variables = {
  a = "a-default-value"
  b = "b-default-value"
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, BundleDefaultsFileName), defaultsContent, tmpFilePerm))

	_, err := Create(t.Context(), CreateOptions{
		Config:     newTestConfig(),
		BundleFile: bundleFile,
		Streams:    iostreams.New(nil, nil, io.Discard),
	})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-defaults-test-"+runtime.GOARCH+"-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	require.True(t, bundleDefinitionContainsLayerTitle(t, entries, "bundle.uds.hcl"))
	require.True(t, bundleDefinitionContainsLayerTitle(t, entries, BundleDefaultsFileName))

	// Verify defaults.uds.hcl content is preserved in the blob.
	var idx struct {
		Manifests []struct {
			Digest       string `json:"digest"`
			ArtifactType string `json:"artifactType"`
		} `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))

	var defManifestBytes []byte
	for _, m := range idx.Manifests {
		if m.ArtifactType == MediaTypeBundleDefinition {
			hex := strings.TrimPrefix(m.Digest, "sha256:")
			defManifestBytes = entries["oci/blobs/sha256/"+hex]
			break
		}
	}
	require.NotNil(t, defManifestBytes)

	var defManifest struct {
		Layers []struct {
			Digest      string            `json:"digest"`
			Annotations map[string]string `json:"annotations"`
		} `json:"layers"`
	}
	require.NoError(t, json.Unmarshal(defManifestBytes, &defManifest))

	for _, l := range defManifest.Layers {
		if l.Annotations["org.opencontainers.image.title"] == BundleDefaultsFileName {
			blob := entries["oci/blobs/sha256/"+strings.TrimPrefix(l.Digest, "sha256:")]
			require.Equal(t, string(defaultsContent), string(blob))
		}
	}
}

func TestCreate_NoDefaultsHCL_NotIncluded(t *testing.T) {
	dir := t.TempDir()

	writeMinimalOCILayout(t, filepath.Join(dir, "localpkg"))

	bundleFile := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "no-defaults-test"
  version = "0.0.1"
}

package "pkg1" {
  source = "localpkg"
}
`), tmpFilePerm))

	_, err := Create(t.Context(), CreateOptions{
		Config:     newTestConfig(),
		BundleFile: bundleFile,
		Streams:    iostreams.New(nil, nil, io.Discard),
	})
	require.NoError(t, err)

	outPath := filepath.Join(dir, "uds-bundle-no-defaults-test-"+runtime.GOARCH+"-0.0.1.tar.zst")
	entries := readTarZstEntries(t, outPath)

	require.True(t, bundleDefinitionContainsLayerTitle(t, entries, "bundle.uds.hcl"))
	require.False(t, bundleDefinitionContainsLayerTitle(t, entries, BundleDefaultsFileName),
		"defaults.uds.hcl should not be in the bundle when the file does not exist")
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
	err = ca.Extract(t.Context(), f, func(_ context.Context, info archives.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		rc, err := info.Open()
		if err != nil {
			return err
		}
		defer func() { _ = rc.Close() }()
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
