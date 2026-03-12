// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// buildTestBlobDir writes a minimal Zarf OCI blob store into a temp dir and
// returns the blob dir path and the initial ociManifest descriptor.
//
// The package has three components:
//   - "core"      required: true
//   - "logging"   required: false (optional)
//   - "monitoring" required: false (optional)
func buildTestBlobDir(t *testing.T) (blobDir string, m ociManifest) {
	t.Helper()

	root := t.TempDir()
	blobDir = filepath.Join(root, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

	reqTrue := true
	reqFalse := false
	pkg := zarfMinimalPackage{
		Components: []zarfMinimalComponent{
			{Name: "core", Required: &reqTrue},
			{Name: "logging", Required: &reqFalse},
			{Name: "monitoring", Required: &reqFalse},
		},
	}
	zarfBytes, err := yaml.Marshal(pkg)
	require.NoError(t, err)
	zarfHash := writeBlob(t, blobDir, zarfBytes)

	configBytes := []byte("{}")
	configHash := writeBlob(t, blobDir, configBytes)

	coreLayerBytes := []byte("core component data")
	coreHash := writeBlob(t, blobDir, coreLayerBytes)

	loggingLayerBytes := []byte("logging component data")
	loggingHash := writeBlob(t, blobDir, loggingLayerBytes)

	monitoringLayerBytes := []byte("monitoring component data")
	monitoringHash := writeBlob(t, blobDir, monitoringLayerBytes)

	layers := []ociDescriptor{
		{
			MediaType:   "application/vnd.zarf.layer.v1.blob",
			Digest:      "sha256:" + zarfHash,
			Size:        int64(len(zarfBytes)),
			Annotations: map[string]string{zarfLayerTitleAnnotation: "zarf.yaml"},
		},
		{
			MediaType:   "application/vnd.zarf.layer.v1.blob",
			Digest:      "sha256:" + coreHash,
			Size:        int64(len(coreLayerBytes)),
			Annotations: map[string]string{zarfLayerTitleAnnotation: "components/core.tar"},
		},
		{
			MediaType:   "application/vnd.zarf.layer.v1.blob",
			Digest:      "sha256:" + loggingHash,
			Size:        int64(len(loggingLayerBytes)),
			Annotations: map[string]string{zarfLayerTitleAnnotation: "components/logging.tar"},
		},
		{
			MediaType:   "application/vnd.zarf.layer.v1.blob",
			Digest:      "sha256:" + monitoringHash,
			Size:        int64(len(monitoringLayerBytes)),
			Annotations: map[string]string{zarfLayerTitleAnnotation: "components/monitoring.tar"},
		},
	}

	im := ociImageManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.zarf.config.v1+json",
		Config: ociDescriptor{
			MediaType: "application/vnd.zarf.config.v1+json",
			Digest:    "sha256:" + configHash,
			Size:      int64(len(configBytes)),
		},
		Layers: layers,
	}
	manifestBytes, err := json.Marshal(im)
	require.NoError(t, err)
	manifestHash := writeBlob(t, blobDir, manifestBytes)

	m = ociManifest{
		MediaType: "application/vnd.zarf.config.v1+json",
		Digest:    "sha256:" + manifestHash,
		Size:      int64(len(manifestBytes)),
	}
	return blobDir, m
}

// writeBlob writes b to blobDir/<sha256hex> and returns the hex digest.
func writeBlob(t *testing.T, blobDir string, b []byte) string {
	t.Helper()
	sum := sha256.Sum256(b)
	hex := hex.EncodeToString(sum[:])
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, hex), b, tmpFilePerm))
	return hex
}

func TestFilterZarfOptionalComponents_EmptyKeepRemovesAllOptionals(t *testing.T) {
	blobDir, m := buildTestBlobDir(t)

	result, err := filterZarfOptionalComponents(blobDir, m, nil)
	require.NoError(t, err)
	assert.NotEqual(t, m.Digest, result.Digest, "filtered manifest should have a new digest")

	h, err := v1.NewHash(result.Digest)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(blobDir, h.Hex))
	require.NoError(t, err)
	var im ociImageManifest
	require.NoError(t, json.Unmarshal(data, &im))

	titles := layerTitles(im.Layers)
	assert.Contains(t, titles, "zarf.yaml")
	assert.Contains(t, titles, "components/core.tar")
	assert.NotContains(t, titles, "components/logging.tar", "unselected optional should be excluded")
	assert.NotContains(t, titles, "components/monitoring.tar", "unselected optional should be excluded")
}

func TestFilterZarfOptionalComponents_NonZarfPackage(t *testing.T) {
	dir := t.TempDir()
	blobDir := filepath.Join(dir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

	// Build a manifest with no zarf.yaml layer.
	configBytes := []byte("{}")
	configHash := writeBlob(t, blobDir, configBytes)
	layerBytes := []byte("some layer data")
	layerHash := writeBlob(t, blobDir, layerBytes)

	im := ociImageManifest{
		SchemaVersion: 2,
		Config:        ociDescriptor{Digest: "sha256:" + configHash, Size: int64(len(configBytes))},
		Layers:        []ociDescriptor{{Digest: "sha256:" + layerHash, Size: int64(len(layerBytes))}},
	}
	manifestBytes, err := json.Marshal(im)
	require.NoError(t, err)
	manifestHash := writeBlob(t, blobDir, manifestBytes)

	m := ociManifest{MediaType: "application/vnd.oci.image.manifest.v1+json", Digest: "sha256:" + manifestHash, Size: int64(len(manifestBytes))}

	result, err := filterZarfOptionalComponents(blobDir, m, nil)
	require.NoError(t, err)
	assert.Equal(t, m, result, "non-Zarf manifest should be returned unchanged")
}

func TestFilterZarfOptionalComponents_KeepOneOptional(t *testing.T) {
	blobDir, m := buildTestBlobDir(t)

	result, err := filterZarfOptionalComponents(blobDir, m, []string{"logging"})
	require.NoError(t, err)
	assert.NotEqual(t, m.Digest, result.Digest, "filtered manifest should have a new digest")

	// Read back the new manifest and verify layers.
	h, err := v1.NewHash(result.Digest)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(blobDir, h.Hex))
	require.NoError(t, err)
	var im ociImageManifest
	require.NoError(t, json.Unmarshal(data, &im))

	titles := layerTitles(im.Layers)
	assert.Contains(t, titles, "zarf.yaml")
	assert.Contains(t, titles, "components/core.tar")
	assert.Contains(t, titles, "components/logging.tar")
	assert.NotContains(t, titles, "components/monitoring.tar", "unselected optional component should be excluded")
}

func TestFilterZarfOptionalComponents_KeepAllOptional(t *testing.T) {
	blobDir, m := buildTestBlobDir(t)

	result, err := filterZarfOptionalComponents(blobDir, m, []string{"logging", "monitoring"})
	require.NoError(t, err)

	h, err := v1.NewHash(result.Digest)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(blobDir, h.Hex))
	require.NoError(t, err)
	var im ociImageManifest
	require.NoError(t, json.Unmarshal(data, &im))

	titles := layerTitles(im.Layers)
	assert.Contains(t, titles, "components/core.tar")
	assert.Contains(t, titles, "components/logging.tar")
	assert.Contains(t, titles, "components/monitoring.tar")
}

func TestFilterZarfOptionalComponents_RequiredComponentError(t *testing.T) {
	blobDir, m := buildTestBlobDir(t)
	_, err := filterZarfOptionalComponents(blobDir, m, []string{"core"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `component "core" is required`)
}

func TestFilterZarfOptionalComponents_UnknownComponentError(t *testing.T) {
	blobDir, m := buildTestBlobDir(t)
	_, err := filterZarfOptionalComponents(blobDir, m, []string{"nonexistent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `does not exist`)
}

func TestZarfComponentNameFromTitle(t *testing.T) {
	tests := []struct {
		title    string
		wantName string
		wantOK   bool
	}{
		{"components/my-pkg.tar", "my-pkg", true},
		{"components/foo-bar.tar", "foo-bar", true},
		{"zarf.yaml", "", false},
		{"checksums.txt", "", false},
		{"components/", "", false},
		{"components/.tar", "", false},
		{"images/index.json", "", false},
	}
	for _, tc := range tests {
		name, ok := zarfComponentNameFromTitle(tc.title)
		assert.Equal(t, tc.wantOK, ok, "title=%q", tc.title)
		assert.Equal(t, tc.wantName, name, "title=%q", tc.title)
	}
}

// writeZarfLikeOCILayout creates a complete OCI image layout directory in layoutDir
// that mimics a single-manifest Zarf package. required and optional specify component
// names. Returns a map of component name → sha256 hex digest of the layer blob.
func writeZarfLikeOCILayout(t *testing.T, layoutDir string, required, optional []string) map[string]string {
	t.Helper()

	blobDir := filepath.Join(layoutDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))

	reqTrue := true
	reqFalse := false
	components := make([]zarfMinimalComponent, 0, len(required)+len(optional))
	for _, name := range required {
		components = append(components, zarfMinimalComponent{Name: name, Required: &reqTrue})
	}
	for _, name := range optional {
		components = append(components, zarfMinimalComponent{Name: name, Required: &reqFalse})
	}

	zarfBytes, err := yaml.Marshal(zarfMinimalPackage{Components: components})
	require.NoError(t, err)
	zarfHash := writeBlob(t, blobDir, zarfBytes)

	configBytes := []byte("{}")
	configHash := writeBlob(t, blobDir, configBytes)

	layers := []ociDescriptor{{
		MediaType:   "application/vnd.zarf.layer.v1.blob",
		Digest:      "sha256:" + zarfHash,
		Size:        int64(len(zarfBytes)),
		Annotations: map[string]string{zarfLayerTitleAnnotation: "zarf.yaml"},
	}}

	compDigests := map[string]string{}
	for _, name := range append(required, optional...) {
		data := []byte(name + " component data")
		h := writeBlob(t, blobDir, data)
		compDigests[name] = h
		layers = append(layers, ociDescriptor{
			MediaType:   "application/vnd.zarf.layer.v1.blob",
			Digest:      "sha256:" + h,
			Size:        int64(len(data)),
			Annotations: map[string]string{zarfLayerTitleAnnotation: "components/" + name + ".tar"},
		})
	}

	im := ociImageManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: ociDescriptor{
			MediaType: "application/vnd.zarf.config.v1+json",
			Digest:    "sha256:" + configHash,
			Size:      int64(len(configBytes)),
		},
		Layers: layers,
	}
	manifestBytes, err := json.Marshal(im)
	require.NoError(t, err)
	manifestHash := writeBlob(t, blobDir, manifestBytes)

	idx := ociIndex{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.index.v1+json",
		Manifests: []ociManifest{{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:" + manifestHash,
			Size:      int64(len(manifestBytes)),
		}},
	}
	idxBytes, err := json.Marshal(idx)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(layoutDir, "index.json"), idxBytes, tmpFilePerm))

	ociLayoutBytes, err := json.Marshal(ociLayout{ImageLayoutVersion: "1.0.0"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(layoutDir, "oci-layout"), ociLayoutBytes, tmpFilePerm))

	return compDigests
}

// layerTitles extracts the org.opencontainers.image.title annotation from each layer.
func layerTitles(layers []ociDescriptor) []string {
	titles := make([]string, 0, len(layers))
	for _, l := range layers {
		titles = append(titles, l.Annotations[zarfLayerTitleAnnotation])
	}
	return titles
}
