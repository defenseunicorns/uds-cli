// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			name: "valid — materializes files and package manifests",
			setup: func(t *testing.T) string {
				hcl := `uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test-bundle" version = "0.1.0" }
package "mypkg" { source = "oci://example.com/pkg:v1" }
`
				return buildBundleArtifact(t, hcl, map[string][]string{
					"mypkg": {"key: value1", "key: value2"},
				}, []string{"mypkg"})
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
				assert.Len(t, extracted.PackageManifests, 1)
				assert.Contains(t, extracted.PackageManifests, "mypkg")
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
				require.NoError(t, ExtractTarZst(t.Context(), iostreams.IOStreams{}, tarPath, unpackDir))

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
				require.NoError(t, WriteTarZst(t.Context(), iostreams.IOStreams{}, corruptPath, unpackDir))
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

func TestBuildPackageManifests(t *testing.T) {
	t.Parallel()
	packageManifest := func(digest godigest.Digest, annotations map[string]string) ocispec.Descriptor {
		return ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest, Annotations: annotations}
	}
	tests := []struct {
		name        string
		manifests   []ocispec.Descriptor
		defIdx      int
		wantDigests map[string]ocispec.Descriptor
		wantErrFrag string
	}{
		{
			name:        "no non-def manifests returns empty map",
			manifests:   []ocispec.Descriptor{{Digest: godigest.Digest("sha256:def")}},
			defIdx:      0,
			wantDigests: map[string]ocispec.Descriptor{},
		},
		{
			name: "single manifest",
			manifests: []ocispec.Descriptor{
				{Digest: godigest.Digest("sha256:def")},
				packageManifest(godigest.Digest("sha256:aaa"), map[string]string{ocispec.AnnotationRefName: "pkg"}),
			},
			defIdx:      0,
			wantDigests: map[string]ocispec.Descriptor{"pkg": packageManifest(godigest.Digest("sha256:aaa"), map[string]string{ocispec.AnnotationRefName: "pkg"})},
		},
		{
			name: "duplicate ref name same digest is idempotent",
			manifests: []ocispec.Descriptor{
				{Digest: godigest.Digest("sha256:def")},
				packageManifest(godigest.Digest("sha256:aaa"), map[string]string{ocispec.AnnotationRefName: "pkg"}),
				packageManifest(godigest.Digest("sha256:aaa"), map[string]string{ocispec.AnnotationRefName: "pkg"}),
			},
			defIdx:      0,
			wantDigests: map[string]ocispec.Descriptor{"pkg": packageManifest(godigest.Digest("sha256:aaa"), map[string]string{ocispec.AnnotationRefName: "pkg"})},
		},
		{
			name: "duplicate ref name different digest returns error",
			manifests: []ocispec.Descriptor{
				{Digest: godigest.Digest("sha256:def")},
				packageManifest(godigest.Digest("sha256:aaa"), map[string]string{ocispec.AnnotationRefName: "pkg"}),
				packageManifest(godigest.Digest("sha256:bbb"), map[string]string{ocispec.AnnotationRefName: "pkg"}),
			},
			defIdx:      0,
			wantErrFrag: "pkg",
		},
		{
			name: "different ref names are indexed separately",
			manifests: []ocispec.Descriptor{
				{Digest: godigest.Digest("sha256:def")},
				packageManifest(godigest.Digest("sha256:aaa"), map[string]string{ocispec.AnnotationRefName: "example.com/pkg-a:v1"}),
				packageManifest(godigest.Digest("sha256:bbb"), map[string]string{ocispec.AnnotationRefName: "example.com/pkg-b:v1"}),
			},
			defIdx: 0,
			wantDigests: map[string]ocispec.Descriptor{
				"example.com/pkg-a:v1": packageManifest(godigest.Digest("sha256:aaa"), map[string]string{ocispec.AnnotationRefName: "example.com/pkg-a:v1"}),
				"example.com/pkg-b:v1": packageManifest(godigest.Digest("sha256:bbb"), map[string]string{ocispec.AnnotationRefName: "example.com/pkg-b:v1"}),
			},
		},
		{
			name: "unsupported media type returns error",
			manifests: []ocispec.Descriptor{
				{Digest: godigest.Digest("sha256:def")},
				{Digest: godigest.Digest("sha256:aaa"), Annotations: map[string]string{ocispec.AnnotationRefName: "pkg"}},
			},
			defIdx:      0,
			wantErrFrag: "unsupported media type",
		},
		{
			name: "missing ref name annotation returns error",
			manifests: []ocispec.Descriptor{
				{Digest: godigest.Digest("sha256:def")},
				packageManifest(godigest.Digest("sha256:aaa"), nil),
			},
			defIdx:      0,
			wantErrFrag: "no org.opencontainers.image.ref.name",
		},
		{
			name: "defIdx manifest is skipped",
			manifests: []ocispec.Descriptor{
				packageManifest(godigest.Digest("sha256:aaa"), map[string]string{ocispec.AnnotationRefName: "pkg"}),
				{Digest: godigest.Digest("sha256:def")},
			},
			defIdx:      1,
			wantDigests: map[string]ocispec.Descriptor{"pkg": packageManifest(godigest.Digest("sha256:aaa"), map[string]string{ocispec.AnnotationRefName: "pkg"})},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			idx := ocispec.Index{Manifests: tt.manifests}
			got, err := buildPackageManifests(idx, tt.defIdx)
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
