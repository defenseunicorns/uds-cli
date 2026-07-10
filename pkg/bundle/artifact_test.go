// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildBundleArtifact creates a minimal but fully valid .tar.zst bundle artifact
// for testing. The artifact contains:
//   - a bundle definition manifest with bundle.uds.hcl and optional values files
//   - one fake package manifest per entry in pkgSources
//
// All blob digests are computed correctly so verifyOCILayoutDigests passes.
// Returns the path to the produced .tar.zst file.
func buildBundleArtifact(t *testing.T, bundleHCL string, valuesFiles map[string][]string, pkgSources []string) string {
	t.Helper()
	return buildBundleArtifactInner(t, bundleHCL, "", valuesFiles, pkgSources, BundleFileName, "zarf.yaml")
}

// buildBundleArtifactWithDefaults is like buildBundleArtifact but also embeds a
// defaults.uds.hcl layer (as a second MediaTypeBundleHCL layer) after bundle.uds.hcl.
// Used to reproduce and guard against the bug where the last HCL layer won BundleDefPath.
func buildBundleArtifactWithDefaults(t *testing.T, bundleHCL, defaultsHCL string, valuesFiles map[string][]string, pkgSources []string) string {
	t.Helper()
	return buildBundleArtifactInner(t, bundleHCL, defaultsHCL, valuesFiles, pkgSources, BundleFileName, "zarf.yaml")
}

func buildBundleArtifactWithTitles(t *testing.T, bundleHCL string, valuesFiles map[string][]string, pkgSources []string, bundleTitle, packageLayerTitle string) string {
	t.Helper()
	return buildBundleArtifactInner(t, bundleHCL, "", valuesFiles, pkgSources, bundleTitle, packageLayerTitle)
}

func buildBundleArtifactInner(t *testing.T, bundleHCL, defaultsHCL string, valuesFiles map[string][]string, pkgSources []string, bundleTitle, packageLayerTitle string) string {
	t.Helper()

	root := t.TempDir()
	ociDir := filepath.Join(root, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, tempDirPerm))
	require.NoError(t, writeOCILayout(filepath.Join(ociDir, "oci-layout")))

	// helper: write blob bytes and return "sha256:<hex>"
	writeBlob := func(data []byte) string {
		sum := sha256.Sum256(data)
		h := hex.EncodeToString(sum[:])
		require.NoError(t, os.WriteFile(filepath.Join(blobDir, h), data, tmpFilePerm))
		return "sha256:" + h
	}

	// Build bundle definition manifest layers
	var defLayers []ociDescriptor

	// bundle.uds.hcl layer
	hclDigest := writeBlob([]byte(bundleHCL))
	defLayers = append(defLayers, ociDescriptor{
		MediaType:   MediaTypeBundleHCL,
		Digest:      hclDigest,
		Size:        int64(len(bundleHCL)),
		Annotations: map[string]string{"org.opencontainers.image.title": bundleTitle},
	})

	// defaults.uds.hcl layer (optional) — appears after bundle.uds.hcl to match real artifacts
	if defaultsHCL != "" {
		defaultsDigest := writeBlob([]byte(defaultsHCL))
		defLayers = append(defLayers, ociDescriptor{
			MediaType:   MediaTypeBundleHCL,
			Digest:      defaultsDigest,
			Size:        int64(len(defaultsHCL)),
			Annotations: map[string]string{"org.opencontainers.image.title": BundleDefaultsFileName},
		})
	}

	// Values layers
	for pkgName, files := range valuesFiles {
		for i, content := range files {
			title := fmt.Sprintf("values/%s/%d.yaml", pkgName, i)
			d := writeBlob([]byte(content))
			defLayers = append(defLayers, ociDescriptor{
				MediaType:   MediaTypeBundleValuesYAML,
				Digest:      d,
				Size:        int64(len(content)),
				Annotations: map[string]string{"org.opencontainers.image.title": title},
			})
		}
	}

	// Bundle definition manifest
	emptyConfigData := []byte("{}")
	emptyConfigDigest := writeBlob(emptyConfigData)
	defManifest := ociImageManifest{
		SchemaVersion: 2,
		Config: ociDescriptor{
			MediaType: "application/vnd.oci.empty.v1+json",
			Digest:    emptyConfigDigest,
			Size:      int64(len(emptyConfigData)),
		},
		Layers: defLayers,
	}
	defManifestData, err := json.Marshal(defManifest)
	require.NoError(t, err)
	defManifestDigest := writeBlob(defManifestData)

	// Package manifests
	var indexManifests []ociManifest
	indexManifests = append(indexManifests, ociManifest{
		MediaType:    "application/vnd.oci.image.manifest.v1+json",
		ArtifactType: MediaTypeBundleDefinition,
		Digest:       defManifestDigest,
		Size:         int64(len(defManifestData)),
	})

	for _, source := range pkgSources {
		pkgData := []byte("fake package: " + source)
		pkgDigest := writeBlob(pkgData)
		pkgManifest := ociImageManifest{
			SchemaVersion: 2,
			Config: ociDescriptor{
				MediaType: "application/vnd.oci.empty.v1+json",
				Digest:    emptyConfigDigest,
				Size:      int64(len(emptyConfigData)),
			},
			Layers: []ociDescriptor{{
				MediaType: MediaTypeZarfLayer,
				Digest:    pkgDigest,
				Size:      int64(len(pkgData)),
			}},
		}
		if packageLayerTitle != "" {
			pkgManifest.Layers[0].Annotations = map[string]string{"org.opencontainers.image.title": packageLayerTitle}
		}
		pkgManifestData, err := json.Marshal(pkgManifest)
		require.NoError(t, err)
		pkgManifestDigest := writeBlob(pkgManifestData)

		refName := source
		if IsOCIReference(source) {
			refName = TrimScheme(source)
		}
		indexManifests = append(indexManifests, ociManifest{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    pkgManifestDigest,
			Size:      int64(len(pkgManifestData)),
			Annotations: map[string]string{
				"org.opencontainers.image.ref.name": refName,
			},
		})
	}

	idx := newBundleIndex(indexManifests, "amd64")
	require.NoError(t, writeOCIIndex(filepath.Join(ociDir, "index.json"), idx))

	// Write the tar.zst
	outPath := filepath.Join(t.TempDir(), "bundle.tar.zst")
	require.NoError(t, writeTarZst(t.Context(), iostreams.IOStreams{}, outPath, root))
	return outPath
}

func TestSafeLayerDestinationPath(t *testing.T) {
	dstDir := t.TempDir()
	cleanDstDir, err := filepath.Abs(filepath.Clean(dstDir))
	require.NoError(t, err)

	tests := []struct {
		name        string
		title       string
		want        string
		wantErrFrag string
	}{
		{
			name:  "simple file",
			title: BundleFileName,
			want:  filepath.Join(dstDir, BundleFileName),
		},
		{
			name:  "nested file",
			title: "values/pkg/0.yaml",
			want:  filepath.Join(dstDir, "values", "pkg", "0.yaml"),
		},
		{
			name:  "cleaned internal traversal",
			title: "values/pkg/../pkg/0.yaml",
			want:  filepath.Join(dstDir, "values", "pkg", "0.yaml"),
		},
		{
			name:        "parent traversal rejected",
			title:       "../../../etc/passwd",
			wantErrFrag: "escapes destination directory",
		},
		{
			name:        "sibling prefix traversal rejected",
			title:       "../" + filepath.Base(dstDir) + "-sibling/file",
			wantErrFrag: "escapes destination directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeLayerDestinationPath(cleanDstDir, dstDir, tt.title)
			if tt.wantErrFrag != "" {
				require.ErrorContains(t, err, tt.wantErrFrag)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractArtifact(t *testing.T) {
	const baseHCL = `uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test" }
package "pkg" { source = "pkg" }
`
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		wantErr     bool
		wantErrFrag string // lowercase substring checked when wantErr is true
		check       func(t *testing.T, extracted *ExtractedBundle, dstDir string)
	}{
		{
			name: "valid — materializes files and package digests",
			setup: func(t *testing.T) string {
				hcl := `uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test-bundle" version = "0.1.0" }
package "mypkg" { source = "oci://example.com/pkg:v1" }
`
				return buildBundleArtifact(t, hcl, map[string][]string{
					"mypkg": {"key: value1", "key: value2"},
				}, []string{"oci://example.com/pkg:v1"})
			},
			check: func(t *testing.T, extracted *ExtractedBundle, dstDir string) {
				assert.Equal(t, dstDir, extracted.Dir)
				assert.Equal(t, filepath.Join(dstDir, "oci"), extracted.OCIDir)
				_, err := os.Stat(filepath.Join(dstDir, BundleFileName))
				require.NoError(t, err, "bundle.uds.hcl should be materialized")
				_, err = os.Stat(filepath.Join(dstDir, "values", "mypkg", "0.yaml"))
				require.NoError(t, err, "values/mypkg/0.yaml should be materialized")
				_, err = os.Stat(filepath.Join(dstDir, "values", "mypkg", "1.yaml"))
				require.NoError(t, err, "values/mypkg/1.yaml should be materialized")
				assert.Len(t, extracted.PackageDigests, 1)
				assert.Contains(t, extracted.PackageDigests, "example.com/pkg:v1")
			},
		},
		{
			// Regression: last-written MediaTypeBundleHCL layer must not win BundleDefPath.
			name: "BundleDefPath is bundle.uds.hcl when defaults.uds.hcl present after it",
			setup: func(t *testing.T) string {
				return buildBundleArtifactWithDefaults(t, baseHCL, `options {}`, nil, []string{"pkg"})
			},
			check: func(t *testing.T, extracted *ExtractedBundle, dstDir string) {
				assert.Equal(t, filepath.Join(dstDir, BundleFileName), extracted.BundleDefPath)
			},
		},
		{
			name: "bundle definition layer title cannot escape destination",
			setup: func(t *testing.T) string {
				return buildBundleArtifactWithTitles(t, baseHCL, nil, []string{"pkg"}, "../../../bundle.uds.hcl", "zarf.yaml")
			},
			wantErr:     true,
			wantErrFrag: "escapes destination directory",
		},
		{
			name:    "non-existent path returns error",
			setup:   func(t *testing.T) string { return "/nonexistent/bundle.tar.zst" },
			wantErr: true,
		},
		{
			name:    "corrupted blob returns digest error",
			wantErr: true,
			setup: func(t *testing.T) string {
				tarPath := buildBundleArtifact(t, baseHCL, nil, []string{"pkg"})

				unpackDir := t.TempDir()
				require.NoError(t, extractTarZst(t.Context(), iostreams.IOStreams{}, tarPath, unpackDir))

				blobDir := filepath.Join(unpackDir, "oci", "blobs", "sha256")
				entries, err := os.ReadDir(blobDir)
				require.NoError(t, err)
				require.NotEmpty(t, entries)

				target := filepath.Join(blobDir, entries[0].Name())
				data, err := os.ReadFile(target)
				require.NoError(t, err)
				data[len(data)-1] ^= 0xFF
				require.NoError(t, os.WriteFile(target, data, tmpFilePerm))

				corruptPath := filepath.Join(t.TempDir(), "corrupt.tar.zst")
				require.NoError(t, writeTarZst(t.Context(), iostreams.IOStreams{}, corruptPath, unpackDir))
				return corruptPath
			},
			wantErrFrag: "digest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tarPath := tt.setup(t)
			dstDir := t.TempDir()
			extracted, err := ExtractArtifact(t.Context(), iostreams.IOStreams{}, tarPath, dstDir)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrFrag != "" {
					assert.Contains(t, strings.ToLower(err.Error()), tt.wantErrFrag)
				}
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, extracted, dstDir)
			}
		})
	}
}

func TestBuildPackageDigests(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		manifests   []ociManifest
		defIdx      int
		wantDigests map[string]string
		wantErrFrag string
	}{
		{
			name:        "no non-def manifests returns empty map",
			manifests:   []ociManifest{{Digest: "sha256:def"}},
			defIdx:      0,
			wantDigests: map[string]string{},
		},
		{
			name: "single manifest",
			manifests: []ociManifest{
				{Digest: "sha256:def"},
				{Digest: "sha256:aaa", Annotations: map[string]string{"org.opencontainers.image.ref.name": "example.com/pkg:v1"}},
			},
			defIdx:      0,
			wantDigests: map[string]string{"example.com/pkg:v1": "sha256:aaa"},
		},
		{
			name: "duplicate ref.name same digest is idempotent",
			manifests: []ociManifest{
				{Digest: "sha256:def"},
				{Digest: "sha256:aaa", Annotations: map[string]string{"org.opencontainers.image.ref.name": "example.com/pkg:v1"}},
				{Digest: "sha256:aaa", Annotations: map[string]string{"org.opencontainers.image.ref.name": "example.com/pkg:v1"}},
			},
			defIdx:      0,
			wantDigests: map[string]string{"example.com/pkg:v1": "sha256:aaa"},
		},
		{
			name: "duplicate ref.name different digest returns error",
			manifests: []ociManifest{
				{Digest: "sha256:def"},
				{Digest: "sha256:aaa", Annotations: map[string]string{"org.opencontainers.image.ref.name": "example.com/pkg:v1"}},
				{Digest: "sha256:bbb", Annotations: map[string]string{"org.opencontainers.image.ref.name": "example.com/pkg:v1"}},
			},
			defIdx:      0,
			wantErrFrag: "example.com/pkg:v1",
		},
		{
			name: "missing ref.name annotation returns error",
			manifests: []ociManifest{
				{Digest: "sha256:def"},
				{Digest: "sha256:aaa"},
			},
			defIdx:      0,
			wantErrFrag: "no org.opencontainers.image.ref.name",
		},
		{
			name: "defIdx manifest is skipped",
			manifests: []ociManifest{
				{Digest: "sha256:aaa", Annotations: map[string]string{"org.opencontainers.image.ref.name": "example.com/pkg:v1"}},
				{Digest: "sha256:def"},
			},
			defIdx:      1,
			wantDigests: map[string]string{"example.com/pkg:v1": "sha256:aaa"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			idx := ociIndex{Manifests: tt.manifests}
			got, err := buildPackageDigests(idx, tt.defIdx)
			if tt.wantErrFrag != "" {
				require.ErrorContains(t, err, tt.wantErrFrag)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantDigests, got)
		})
	}
}

func TestValuesFilesByPackage(t *testing.T) {
	const baseHCL = `uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test" }
package "mypkg" { source = "mypkg" }
`
	tests := []struct {
		name        string
		valuesFiles map[string][]string
		check       func(t *testing.T, result map[string][]string)
	}{
		{
			name: "numeric ordering — 11 files sorted correctly",
			valuesFiles: func() map[string][]string {
				files := make([]string, 11)
				for i := range files {
					files[i] = fmt.Sprintf("val%d: content", i)
				}
				return map[string][]string{"mypkg": files}
			}(),
			check: func(t *testing.T, result map[string][]string) {
				paths := result["mypkg"]
				require.Len(t, paths, 11)
				for i, p := range paths {
					assert.True(t, strings.HasSuffix(p, fmt.Sprintf("%d.yaml", i)),
						"path %d should be %d.yaml, got %s", i, i, filepath.Base(p))
				}
			},
		},
		{
			name:        "no values files returns empty map",
			valuesFiles: nil,
			check: func(t *testing.T, result map[string][]string) {
				assert.Empty(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tarPath := buildBundleArtifact(t, baseHCL, tt.valuesFiles, []string{"mypkg"})
			extracted, err := ExtractArtifact(t.Context(), iostreams.IOStreams{}, tarPath, t.TempDir())
			require.NoError(t, err)

			result, err := extracted.ValuesFilesByPackage()
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}
