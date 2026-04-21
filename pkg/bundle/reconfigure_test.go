// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	oras "oras.land/oras-go/v2"
	oraci "oras.land/oras-go/v2/content/oci"
)

func TestReconfiguredFileOutputName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		suffix string
		want   string
	}{
		{
			name:   "inserts before arm64 and version",
			source: "uds-bundle-defaults-test-arm64-0.1.0.tar.zst",
			suffix: "-smoke-test",
			want:   "uds-bundle-defaults-test-smoke-test-arm64-0.1.0.tar.zst",
		},
		{
			name:   "inserts before amd64 and version",
			source: "uds-bundle-my-bundle-amd64-2.3.4.tar.zst",
			suffix: "-custom",
			want:   "uds-bundle-my-bundle-custom-amd64-2.3.4.tar.zst",
		},
		{
			name:   "name contains arch-like substring - anchors on last occurrence",
			source: "uds-bundle-arm64-app-arm64-1.0.0.tar.zst",
			suffix: "-x",
			want:   "uds-bundle-arm64-app-x-arm64-1.0.0.tar.zst",
		},
		{
			name:   "no recognized arch - falls back to appending",
			source: "uds-bundle-custom-name.tar.zst",
			suffix: "-reconfigured",
			want:   "uds-bundle-custom-name-reconfigured.tar.zst",
		},
		{
			name:   "user-renamed file without arch - falls back to appending",
			source: "my-bundle.tar.zst",
			suffix: "-v2",
			want:   "my-bundle-v2.tar.zst",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := reconfiguredFileOutputName(tc.source, tc.suffix)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSpliceHCLName(t *testing.T) {
	tests := []struct {
		name      string
		hcl       string
		suffix    string
		wantErr   string
		wantCheck func(t *testing.T, result []byte)
	}{
		{
			name: "simple string literal",
			hcl: `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "my-bundle"
  version = "1.0.0"
}
`,
			suffix: "-reconfigured",
			wantCheck: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"my-bundle-reconfigured"`)
				assert.Contains(t, string(result), `version = "1.0.0"`)
				assert.Contains(t, string(result), `bundle_api_version = "uds.dev/v1alpha1"`)
			},
		},
		{
			name: "custom suffix",
			hcl: `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "core"
}
`,
			suffix: "-il5",
			wantCheck: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"core-il5"`)
			},
		},
		{
			name: "preserves comments and formatting",
			hcl: `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
# This is an important comment
metadata {
  name = "my-bundle" # bundle name
  version = "1.0.0"
}
`,
			suffix: "-prod",
			wantCheck: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "# This is an important comment")
				assert.Contains(t, string(result), "# bundle name")
				assert.Contains(t, string(result), `"my-bundle-prod"`)
			},
		},
		{
			name: "template expression with interpolation",
			hcl: `locals {
  prefix = "uds"
}
uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "${local.prefix}-core"
}
`,
			suffix: "-reconfigured",
			wantCheck: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"${local.prefix}-core-reconfigured"`)
			},
		},
		{
			name: "bare local reference wrapped into template",
			hcl: `locals {
  bundle_name = "my-bundle"
}
uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = local.bundle_name
}
`,
			suffix: "-reconfigured",
			wantCheck: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"${local.bundle_name}-reconfigured"`)
				// Verify the rest is preserved.
				assert.Contains(t, string(result), `bundle_name = "my-bundle"`)
			},
		},
		{
			name: "bare nested local reference",
			hcl: `locals {
  meta = {
    name = "nested-bundle"
  }
}
uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = local.meta.name
}
`,
			suffix: "-staging",
			wantCheck: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"${local.meta.name}-staging"`)
			},
		},
		{
			name: "already reconfigured bundle (chained reconfigure)",
			hcl: `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "core-il5"
}
`,
			suffix: "-site-a",
			wantCheck: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"core-il5-site-a"`)
			},
		},
		{
			name: "no metadata block",
			hcl: `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
`,
			suffix:  "-reconfigured",
			wantErr: "not found",
		},
		{
			name: "metadata block without name attribute",
			hcl: `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  version = "1.0.0"
}
`,
			suffix:  "-reconfigured",
			wantErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := spliceHCLName([]byte(tt.hcl), tt.suffix)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantCheck != nil {
				tt.wantCheck(t, result)
			}
		})
	}
}

func TestRebuildDefinitionManifest(t *testing.T) {
	newHCL := ociDescriptor{
		MediaType:   MediaTypeBundleHCL,
		Digest:      "sha256:newhcl",
		Size:        15,
		Annotations: map[string]string{ocispec.AnnotationTitle: "bundle.uds.hcl"},
	}
	newDefaults := ociDescriptor{
		MediaType:   MediaTypeBundleHCL,
		Digest:      "sha256:newdefaults",
		Size:        25,
		Annotations: map[string]string{ocispec.AnnotationTitle: BundleDefaultsFileName},
	}

	tests := []struct {
		name     string
		original ociImageManifest
		check    func(t *testing.T, rebuilt ociImageManifest)
	}{
		{
			name: "replaces existing defaults and HCL layers",
			original: ociImageManifest{
				SchemaVersion: 2,
				MediaType:     "application/vnd.oci.image.manifest.v1+json",
				ArtifactType:  MediaTypeBundleDefinition,
				Config:        ociDescriptor{Digest: "sha256:cfg", Size: 2},
				Layers: []ociDescriptor{
					{MediaType: MediaTypeBundleHCL, Digest: "sha256:oldhcl", Size: 10, Annotations: map[string]string{ocispec.AnnotationTitle: "bundle.uds.hcl"}},
					{MediaType: MediaTypeBundleHCL, Digest: "sha256:olddefaults", Size: 20, Annotations: map[string]string{ocispec.AnnotationTitle: BundleDefaultsFileName}},
				},
				Annotations: map[string]string{ocispec.AnnotationCreated: "1970-01-01T00:00:00Z"},
			},
			check: func(t *testing.T, rebuilt ociImageManifest) {
				require.Len(t, rebuilt.Layers, 2)
				assert.Equal(t, "sha256:newhcl", rebuilt.Layers[0].Digest)
				assert.Equal(t, "sha256:newdefaults", rebuilt.Layers[1].Digest)
			},
		},
		{
			name: "inserts defaults when original had none",
			original: ociImageManifest{
				SchemaVersion: 2,
				ArtifactType:  MediaTypeBundleDefinition,
				Config:        ociDescriptor{Digest: "sha256:cfg", Size: 2},
				Layers: []ociDescriptor{
					{MediaType: MediaTypeBundleHCL, Digest: "sha256:oldhcl", Size: 10, Annotations: map[string]string{ocispec.AnnotationTitle: "bundle.uds.hcl"}},
					{MediaType: MediaTypeBundleValuesYAML, Digest: "sha256:vals", Size: 30, Annotations: map[string]string{ocispec.AnnotationTitle: "values/pkg/0.yaml"}},
				},
			},
			check: func(t *testing.T, rebuilt ociImageManifest) {
				require.Len(t, rebuilt.Layers, 3)
				assert.Equal(t, "sha256:newhcl", rebuilt.Layers[0].Digest)
				assert.Equal(t, "sha256:newdefaults", rebuilt.Layers[1].Digest)
				assert.Equal(t, "sha256:vals", rebuilt.Layers[2].Digest)
			},
		},
		{
			name: "preserves values file layers",
			original: ociImageManifest{
				SchemaVersion: 2,
				Config:        ociDescriptor{Digest: "sha256:cfg", Size: 2},
				Layers: []ociDescriptor{
					{MediaType: MediaTypeBundleHCL, Digest: "sha256:oldhcl", Size: 10, Annotations: map[string]string{ocispec.AnnotationTitle: "bundle.uds.hcl"}},
					{MediaType: MediaTypeBundleHCL, Digest: "sha256:olddefaults", Size: 20, Annotations: map[string]string{ocispec.AnnotationTitle: BundleDefaultsFileName}},
					{MediaType: MediaTypeBundleValuesYAML, Digest: "sha256:v1", Size: 30, Annotations: map[string]string{ocispec.AnnotationTitle: "values/pkg1/0.yaml"}},
					{MediaType: MediaTypeBundleValuesYAML, Digest: "sha256:v2", Size: 40, Annotations: map[string]string{ocispec.AnnotationTitle: "values/pkg2/0.yaml"}},
				},
			},
			check: func(t *testing.T, rebuilt ociImageManifest) {
				require.Len(t, rebuilt.Layers, 4)
				assert.Equal(t, "sha256:v1", rebuilt.Layers[2].Digest)
				assert.Equal(t, "sha256:v2", rebuilt.Layers[3].Digest)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := rebuildDefinitionManifest(tt.original, newDefaults, newHCL, "sha256:origmanifest")
			require.NoError(t, err)

			var rebuilt ociImageManifest
			require.NoError(t, json.Unmarshal(result, &rebuilt))

			// Common assertions across all cases.
			assert.Equal(t, "sha256:origmanifest", rebuilt.Annotations[AnnotationReconfiguredFrom])
			assert.Equal(t, "1970-01-01T00:00:00Z", rebuilt.Annotations[ocispec.AnnotationCreated])

			tt.check(t, rebuilt)
		})
	}
}

// --- Test helpers for reconfigure ---

// extractHCLFromBundle extracts the bundle.uds.hcl content from bundle tarball entries.
func extractHCLFromBundle(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	idx := parseIndexJSON(t, entries)
	defEntry, _, err := findBundleDefinitionEntry(idx)
	require.NoError(t, err)

	manifestBytes := entries["oci/blobs/sha256/"+strings.TrimPrefix(defEntry.Digest, "sha256:")]
	var manifest ociImageManifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))

	for _, l := range manifest.Layers {
		if l.Annotations[ocispec.AnnotationTitle] == "bundle.uds.hcl" {
			return entries["oci/blobs/sha256/"+strings.TrimPrefix(l.Digest, "sha256:")]
		}
	}
	t.Fatal("bundle.uds.hcl layer not found")
	return nil
}

// parseIndexJSON parses the oci/index.json from bundle tarball entries.
func parseIndexJSON(t *testing.T, entries map[string][]byte) ociIndex {
	t.Helper()
	var idx ociIndex
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))
	return idx
}

// createTestBundle creates a bundle from HCL content with an optional defaults file.
// Returns the path to the output tarball.
func createTestBundle(t *testing.T, bundleHCL string, defaultsHCL string) string {
	t.Helper()
	dir := t.TempDir()

	writeMinimalOCILayout(t, filepath.Join(dir, "localpkg"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bundle.uds.hcl"), []byte(bundleHCL), tmpFilePerm))
	if defaultsHCL != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, BundleDefaultsFileName), []byte(defaultsHCL), tmpFilePerm))
	}

	result, err := Create(context.Background(), CreateOptions{
		Config:     newTestConfig(),
		BundleFile: filepath.Join(dir, "bundle.uds.hcl"),
		Out:        io.Discard,
	})
	require.NoError(t, err)
	return result.OutputPath
}

// writeDefaultsFile writes a defaults file and returns its path.
func writeDefaultsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "new-defaults.uds.hcl")
	require.NoError(t, os.WriteFile(path, []byte(content), tmpFilePerm))
	return path
}

// runLocalReconfigure runs the localReconfigurer with a temp output dir and returns the result.
func runLocalReconfigure(t *testing.T, source string, defaultsPath string, suffix string) (*ReconfigureResult, error) {
	t.Helper()
	r := &localReconfigurer{}
	return r.Reconfigure(context.Background(), ReconfigureOptions{
		Source:       source,
		DefaultsFile: defaultsPath,
		Suffix:       suffix,
		OutputDir:    t.TempDir(),
		Options:      ConfigOptions{TmpDir: t.TempDir()},
	})
}

func TestLocalReconfigure_HappyPath(t *testing.T) {
	t.Parallel()
	tarball := createTestBundle(t, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "reconfig-test"
  version = "1.0.0"
}
package "pkg1" {
  source = "localpkg"
}
`, `variables = { old = "value" }`)

	defaultsPath := writeDefaultsFile(t, `variables = { new = "replaced" }`)

	result, err := runLocalReconfigure(t, tarball, defaultsPath, "-custom")
	require.NoError(t, err)
	require.NotEmpty(t, result.OutputPath)
	assert.Contains(t, result.OutputPath, "-custom")

	entries := readTarZstEntries(t, result.OutputPath)

	// Verify defaults layer exists.
	assert.True(t, bundleDefinitionContainsLayerTitle(t, entries, BundleDefaultsFileName))

	// Verify HCL name was updated.
	hclContent := extractHCLFromBundle(t, entries)
	assert.Contains(t, string(hclContent), `"reconfig-test-custom"`)

	// Verify provenance annotation.
	idx := parseIndexJSON(t, entries)
	defEntry, _, err := findBundleDefinitionEntry(idx)
	require.NoError(t, err)
	manifestBytes := entries["oci/blobs/sha256/"+strings.TrimPrefix(defEntry.Digest, "sha256:")]
	var manifest ociImageManifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	assert.Contains(t, manifest.Annotations, AnnotationReconfiguredFrom)
}

func TestLocalReconfigure_OutputAlreadyExists(t *testing.T) {
	t.Parallel()
	tarball := createTestBundle(t, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "exists-test"
  version = "1.0.0"
}
package "pkg1" {
  source = "localpkg"
}
`, "")

	defaultsPath := writeDefaultsFile(t, `variables = { a = "b" }`)

	outDir := t.TempDir()

	// Pre-create the output file.
	expectedName := reconfiguredFileOutputName(filepath.Base(tarball), "-reconfigured")
	require.NoError(t, os.WriteFile(filepath.Join(outDir, expectedName), []byte("exists"), tmpFilePerm))

	r := &localReconfigurer{}
	_, err := r.Reconfigure(context.Background(), ReconfigureOptions{
		Source:       tarball,
		DefaultsFile: defaultsPath,
		Suffix:       "-reconfigured",
		OutputDir:    outDir,
		Options:      ConfigOptions{TmpDir: t.TempDir()},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestLocalReconfigure_InsertsDefaultsWhenOriginalHadNone(t *testing.T) {
	t.Parallel()
	tarball := createTestBundle(t, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "no-defaults"
  version = "1.0.0"
}
package "pkg1" {
  source = "localpkg"
}
`, "") // No defaults.

	defaultsPath := writeDefaultsFile(t, `variables = { inserted = true }`)

	result, err := runLocalReconfigure(t, tarball, defaultsPath, "-reconfigured")
	require.NoError(t, err)

	entries := readTarZstEntries(t, result.OutputPath)
	assert.True(t, bundleDefinitionContainsLayerTitle(t, entries, BundleDefaultsFileName),
		"defaults.uds.hcl should be inserted when original had none")
}

func TestLocalReconfigure_NonBundleTarball(t *testing.T) {
	t.Parallel()

	// Create a tarball that's not a bundle.
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "not-a-bundle.txt"), []byte("nope"), tmpFilePerm))
	tarball := filepath.Join(t.TempDir(), "fake.tar.zst")
	require.NoError(t, writeTarZst(context.Background(), tarball, srcDir))

	defaultsPath := writeDefaultsFile(t, `variables = { a = "b" }`)

	_, err := runLocalReconfigure(t, tarball, defaultsPath, "-reconfigured")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading index.json")
}

func TestLocalReconfigure_RejectsNonTarZstSource(t *testing.T) {
	t.Parallel()
	defaultsPath := writeDefaultsFile(t, `variables = { a = "b" }`)

	_, err := runLocalReconfigure(t, "/path/to/bundle.zip", defaultsPath, "-reconfigured")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source must be a .tar.zst file")
}

// pushTestBundle creates a bundle and pushes it to the given ORAS store.
func pushTestBundle(t *testing.T, store oras.Target, bundleHCL string, defaultsHCL string, ref string) {
	t.Helper()
	tarball := createTestBundle(t, bundleHCL, defaultsHCL)
	_, err := Push(context.Background(), PushOptions{
		BundleTarball: tarball,
		OCIReference:  ref,
		Options:       ConfigOptions{TmpDir: t.TempDir()},
		remoteRepo:    store,
	})
	require.NoError(t, err)
}

func TestOCIReconfigure_HappyPath(t *testing.T) {
	t.Parallel()

	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	pushTestBundle(t, store, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "oci-reconfig"
  version = "1.0.0"
}
package "pkg1" {
  source = "localpkg"
}
`, `variables = { old = "value" }`, "example.com/test/oci-reconfig:v1.0.0")

	defaultsPath := writeDefaultsFile(t, `variables = { new = "replaced" }`)

	r := &ociReconfigurer{}
	result, err := r.Reconfigure(context.Background(), ReconfigureOptions{
		Source:       "oci://example.com/test/oci-reconfig:v1.0.0",
		DefaultsFile: defaultsPath,
		Suffix:       "-prod",
		Options:      ConfigOptions{TmpDir: t.TempDir(), PlainHTTP: true},
		remoteRepo:   store,
	})
	require.NoError(t, err)
	assert.Equal(t, "oci://example.com/test/oci-reconfig:v1.0.0-prod", result.OCIReference)

	// Verify new tag exists.
	_, err = store.Resolve(t.Context(), "v1.0.0-prod")
	require.NoError(t, err, "reconfigured tag should exist")

	// Original tag still exists.
	_, err = store.Resolve(t.Context(), "v1.0.0")
	require.NoError(t, err, "original tag should still exist")
}

func TestOCIReconfigure_TargetTagAlreadyExists(t *testing.T) {
	t.Parallel()

	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	bundleHCL := `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "oci-exists"
  version = "1.0.0"
}
package "pkg1" {
  source = "localpkg"
}
`
	// Push under both the original and target tags.
	pushTestBundle(t, store, bundleHCL, "", "example.com/test/oci-exists:v1.0.0")
	pushTestBundle(t, store, bundleHCL, "", "example.com/test/oci-exists:v1.0.0-reconfigured")

	defaultsPath := writeDefaultsFile(t, `variables = { a = "b" }`)

	r := &ociReconfigurer{}
	_, err = r.Reconfigure(context.Background(), ReconfigureOptions{
		Source:       "oci://example.com/test/oci-exists:v1.0.0",
		DefaultsFile: defaultsPath,
		Suffix:       "-reconfigured",
		Options:      ConfigOptions{TmpDir: t.TempDir(), PlainHTTP: true},
		remoteRepo:   store,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestOCIReconfigure_SourceTagNotFound(t *testing.T) {
	t.Parallel()

	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	defaultsPath := writeDefaultsFile(t, `variables = { a = "b" }`)

	r := &ociReconfigurer{}
	_, err = r.Reconfigure(context.Background(), ReconfigureOptions{
		Source:       "oci://example.com/test/missing:v1.0.0",
		DefaultsFile: defaultsPath,
		Suffix:       "-reconfigured",
		Options:      ConfigOptions{TmpDir: t.TempDir(), PlainHTTP: true},
		remoteRepo:   store,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving")
}

func TestOCIReconfigure_DigestReferenceRejected(t *testing.T) {
	t.Parallel()

	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	defaultsPath := writeDefaultsFile(t, `variables = { a = "b" }`)

	r := &ociReconfigurer{}
	_, err = r.Reconfigure(context.Background(), ReconfigureOptions{
		Source:       "oci://example.com/test/bundle@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
		DefaultsFile: defaultsPath,
		Suffix:       "-reconfigured",
		Options:      ConfigOptions{TmpDir: t.TempDir(), PlainHTTP: true},
		remoteRepo:   store,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag reference")
}

func TestReconfigure_DispatchesLocal(t *testing.T) {
	t.Parallel()
	tarball := createTestBundle(t, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "dispatch-test"
  version = "1.0.0"
}
package "pkg1" {
  source = "localpkg"
}
`, "")

	defaultsPath := writeDefaultsFile(t, `variables = { a = "b" }`)

	result, err := Reconfigure(context.Background(), ReconfigureOptions{
		Source:       tarball,
		DefaultsFile: defaultsPath,
		Suffix:       "-reconfigured",
		OutputDir:    t.TempDir(),
		Options:      ConfigOptions{TmpDir: t.TempDir()},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.OutputPath)
	assert.Empty(t, result.OCIReference)
}

func TestReconfigure_InvalidDefaults(t *testing.T) {
	t.Parallel()
	badDefaultsPath := filepath.Join(t.TempDir(), "defaults.uds.hcl")
	require.NoError(t, os.WriteFile(badDefaultsPath, []byte(`not_variables = "bad"`), tmpFilePerm))

	_, err := Reconfigure(context.Background(), ReconfigureOptions{
		Source:       "/some/bundle.tar.zst",
		DefaultsFile: badDefaultsPath,
		Suffix:       "-reconfigured",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not_variables")
}

func TestReconfigure_SuffixValidation(t *testing.T) {
	t.Parallel()
	defaultsPath := writeDefaultsFile(t, `variables = { a = "b" }`)

	tests := []struct {
		name   string
		suffix string
		valid  bool
	}{
		{"standard suffix", "-reconfigured", true},
		{"short suffix", "-il5", true},
		{"dots and underscores", "-v1.2_beta", true},
		{"multiple hyphens", "-site-a-prod", true},
		{"empty", "", false},
		{"no leading hyphen", "reconfigured", false},
		{"HCL injection", "-${file(\"/etc/shadow\")}", false},
		{"spaces", "-has spaces", false},
		{"special chars", "-foo@bar", false},
		{"slash", "-foo/bar", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Reconfigure(context.Background(), ReconfigureOptions{
				Source:       "/some/bundle.tar.zst",
				DefaultsFile: defaultsPath,
				Suffix:       tt.suffix,
			})
			if tt.valid {
				// Valid suffixes will fail later (source doesn't exist), but not on suffix validation.
				if err != nil {
					assert.NotContains(t, err.Error(), "invalid suffix")
				}
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid suffix")
			}
		})
	}
}
