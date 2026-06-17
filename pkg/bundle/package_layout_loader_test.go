// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractedArtifactPackageLayoutLoader_LoadPackageLayout(t *testing.T) {
	tests := []struct {
		name         string
		pkg          *Package
		digests      map[string]string
		wantNotFound bool
	}{
		{
			name:         "OCI source key uses TrimScheme",
			pkg:          &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"},
			digests:      map[string]string{"example.com/pkg:v1": "sha256:aaa"},
			wantNotFound: false, // found; fails later on blob read
		},
		{
			name:         "local source key uses pkg.Name",
			pkg:          &Package{Name: "localpkg", Source: "./local-path"},
			digests:      map[string]string{"localpkg": "sha256:bbb"},
			wantNotFound: false, // found; fails later on blob read
		},
		{
			name:         "missing OCI source",
			pkg:          &Package{Name: "other", Source: "oci://example.com/other:v1"},
			digests:      map[string]string{"example.com/pkg:v1": "sha256:aaa"},
			wantNotFound: true,
		},
		{
			// Local source not in PackageDigests falls back to directory staging;
			// "./absent" does not exist so staging fails (not an index-not-found error).
			name:         "missing local source falls back to directory staging",
			pkg:          &Package{Name: "absent", Source: "./absent"},
			digests:      map[string]string{"localpkg": "sha256:bbb"},
			wantNotFound: false,
		},
		{
			name:         "empty digests map",
			pkg:          &Package{Name: "anypkg", Source: "oci://example.com/any:v1"},
			digests:      map[string]string{},
			wantNotFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := &ExtractedArtifactPackageLayoutLoader{
				OCIDir:         t.TempDir(),
				PackageDigests: tt.digests,
			}
			_, err := loader.LoadPackageLayout(context.Background(), tt.pkg, t.TempDir(), LoadOptions{})
			require.Error(t, err)
			if tt.wantNotFound {
				assert.Contains(t, err.Error(), "not found in bundle artifact index")
			} else {
				assert.NotContains(t, err.Error(), "not found in bundle artifact index")
			}
		})
	}
}

func TestExtractedArtifactPackageLayoutLoader_StagesFiles(t *testing.T) {
	hcl := `uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "stage-test" }
package "mypkg" { source = "oci://example.com/pkg:v1" }
`
	tarPath := buildBundleArtifact(t, hcl, nil, []string{"oci://example.com/pkg:v1"})

	extracted, err := ExtractArtifact(context.Background(), iostreams.IOStreams{}, tarPath, t.TempDir())
	require.NoError(t, err)

	loader := &ExtractedArtifactPackageLayoutLoader{
		OCIDir:         extracted.OCIDir,
		PackageDigests: extracted.PackageDigests,
	}

	pkg := &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"}
	dstDir := t.TempDir()

	// LoadPackage fails at layout.LoadFromDir since fixture has fake content,
	// not a valid Zarf package. Confirm layer files were staged before that failure.
	_, err = loader.LoadPackageLayout(context.Background(), pkg, dstDir, LoadOptions{})
	require.Error(t, err)

	assert.FileExists(t, filepath.Join(dstDir, "zarf.yaml"), "zarf.yaml layer should be staged before LoadFromDir is called")
}

func TestExtractedArtifactPackageLayoutLoader_RejectsEscapingLayerTitle(t *testing.T) {
	hcl := `uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "stage-test" }
package "mypkg" { source = "oci://example.com/pkg:v1" }
`
	const escapingTitle = "../../../escaped-zarf.yaml"
	tarPath := buildBundleArtifactWithTitles(t, hcl, nil, []string{"oci://example.com/pkg:v1"}, BundleFileName, escapingTitle)

	extracted, err := ExtractArtifact(context.Background(), iostreams.IOStreams{}, tarPath, t.TempDir())
	require.NoError(t, err)

	loader := &ExtractedArtifactPackageLayoutLoader{
		OCIDir:         extracted.OCIDir,
		PackageDigests: extracted.PackageDigests,
	}
	dstDir := t.TempDir()
	escapedPath := filepath.Clean(filepath.Join(dstDir, filepath.FromSlash(escapingTitle)))

	_, err = loader.LoadPackageLayout(context.Background(), &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"}, dstDir, LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes destination directory")
	assert.NoFileExists(t, escapedPath)
}

func TestExtractedArtifactPackageLayoutLoader_RejectsMissingLayerTitle(t *testing.T) {
	hcl := `uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "stage-test" }
package "mypkg" { source = "oci://example.com/pkg:v1" }
`
	tarPath := buildBundleArtifactWithTitles(t, hcl, nil, []string{"oci://example.com/pkg:v1"}, BundleFileName, "")

	extracted, err := ExtractArtifact(context.Background(), iostreams.IOStreams{}, tarPath, t.TempDir())
	require.NoError(t, err)

	loader := &ExtractedArtifactPackageLayoutLoader{
		OCIDir:         extracted.OCIDir,
		PackageDigests: extracted.PackageDigests,
	}
	dstDir := t.TempDir()

	_, err = loader.LoadPackageLayout(context.Background(), &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"}, dstDir, LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `manifest for package "mypkg" missing title annotation on layer`)

	entries, readErr := os.ReadDir(dstDir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "layer without title should not be staged")
}

func TestCopyFileContents_ObservesCanceledContext(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")
	require.NoError(t, os.WriteFile(src, []byte("contents"), 0o600))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := copyFileContents(ctx, src, dst)
	require.ErrorIs(t, err, context.Canceled)
	assert.NoFileExists(t, dst)
}

func TestCtxReader_ObservesCancellationBetweenReads(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	reader := &ctxReader{ctx: ctx, r: strings.NewReader("contents")}
	buf := make([]byte, 1)

	n, err := reader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	cancel()

	n, err = reader.Read(buf)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, n)
}

func TestExtractedArtifactPackageLayoutLoader_DirectoryFallback(t *testing.T) {
	t.Run("stages files from pkg.Source when not in PackageDigests", func(t *testing.T) {
		pkgDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "zarf.yaml"), []byte("kind: ZarfPackageConfig\nmetadata:\n  name: test\n"), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, "components"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "components", "test.tar"), []byte("dummy"), 0o600))

		dstDir := t.TempDir()
		loader := &ExtractedArtifactPackageLayoutLoader{PackageDigests: map[string]string{}}
		pkg := &Package{Name: "mypkg", Source: pkgDir}

		// Fails at layout.LoadFromDir since fixture has incomplete content,
		// but staging side effects should be visible.
		_, err := loader.LoadPackageLayout(context.Background(), pkg, dstDir, LoadOptions{})
		require.Error(t, err)

		assert.FileExists(t, filepath.Join(dstDir, "zarf.yaml"), "zarf.yaml should be staged")
		assert.DirExists(t, filepath.Join(dstDir, "components"), "components dir should be staged")
		assert.FileExists(t, filepath.Join(dstDir, "components", "test.tar"), "test.tar should be staged")

		// Source directory is preserved.
		assert.DirExists(t, pkgDir, "source dir should be preserved after staging")
		assert.FileExists(t, filepath.Join(pkgDir, "zarf.yaml"), "original zarf.yaml should be preserved")
		assert.FileExists(t, filepath.Join(pkgDir, "components", "test.tar"), "original test.tar should be preserved")
	})

	t.Run("OCI source not in PackageDigests returns error (not dir fallback)", func(t *testing.T) {
		loader := &ExtractedArtifactPackageLayoutLoader{PackageDigests: map[string]string{}}
		pkg := &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"}
		_, err := loader.LoadPackageLayout(context.Background(), pkg, t.TempDir(), LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found in bundle artifact index")
	})
}

func TestValuesFilesByPackage_AfterArtifactExtract(t *testing.T) {
	files := make([]string, 11)
	for i := range files {
		files[i] = "key: value"
	}
	hcl := `uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "values-test" }
package "mypkg" { source = "mypkg" }
`
	tarPath := buildBundleArtifact(t, hcl, map[string][]string{"mypkg": files}, []string{"mypkg"})

	extracted, err := ExtractArtifact(context.Background(), iostreams.IOStreams{}, tarPath, t.TempDir())
	require.NoError(t, err)

	result, err := extracted.ValuesFilesByPackage()
	require.NoError(t, err)

	paths := result["mypkg"]
	require.Len(t, paths, 11)
	for i, p := range paths {
		assert.Equal(t, filepath.Join(extracted.Dir, "values", "mypkg", fmt.Sprintf("%d.yaml", i)), p)
	}
}
