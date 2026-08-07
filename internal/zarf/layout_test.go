// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/bundlehcl"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
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
			_, err := loader.LoadPackageLayout(t.Context(), tt.pkg, t.TempDir(), LoadOptions{})
			require.Error(t, err)
			if tt.wantNotFound {
				assert.Contains(t, err.Error(), "not found in bundle artifact index")
			} else {
				assert.NotContains(t, err.Error(), "not found in bundle artifact index")
			}
		})
	}
}

func TestArtifactPackageLayoutOptionsNeverVerifies(t *testing.T) {
	filter := BuildComponentFilter([]string{"optional"})
	opts := artifactPackageLayoutOptions(filter, true)

	assert.Same(t, filter, opts.Filter)
	assert.True(t, opts.IsPartial)
	assert.Equal(t, layout.VerifyNever, opts.VerificationStrategy)
}

func TestSourcePackageLayoutLoaderAdvisoryVerification(t *testing.T) {
	falseValue := false
	tests := []struct {
		name         string
		verification *PackageSignatureVerification
		wantWarning  string
	}{
		{
			name:         "verification failure warns and continues",
			verification: &PackageSignatureVerification{PublicKey: "test public key"},
			wantWarning:  "would fail bundle create",
		},
		{
			name:        "missing create policy warns and continues",
			wantWarning: "would fail bundle create",
		},
		{
			name:         "explicit bypass warns and continues",
			verification: &PackageSignatureVerification{Verify: &falseValue},
			wantWarning:  "unverified package",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkgDir := t.TempDir()
			writeValidUnsignedZarfPackage(t, pkgDir)
			streams, _, out, errOut := iostreams.NewTestIOStreams()
			streams = logger.Bind(streams, "info")
			loader := &SourcePackageLayoutLoader{
				configOpts: ConfigOptions{Architecture: "amd64", TmpDir: t.TempDir()},
			}

			pkgLayout, err := loader.LoadPackageLayout(t.Context(), &Package{
				Name:                  "test",
				Source:                pkgDir,
				SignatureVerification: tt.verification,
			}, t.TempDir(), LoadOptions{Streams: streams})
			require.NoError(t, err)
			require.NotNil(t, pkgLayout)
			assert.Contains(t, out.String()+errOut.String(), tt.wantWarning)
			require.NoError(t, pkgLayout.Cleanup())
		})
	}
}

func TestExtractedArtifactPackageLayoutLoader_StagesFiles(t *testing.T) {
	loader := newArtifactPackageLayoutLoader(t, "zarf.yaml")

	pkg := &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"}
	dstDir := t.TempDir()

	// LoadPackage fails at layout.LoadFromDir since fixture has fake content,
	// not a valid Zarf package. Confirm layer files were staged before that failure.
	_, err := loader.LoadPackageLayout(t.Context(), pkg, dstDir, LoadOptions{})
	require.Error(t, err)

	assert.FileExists(t, filepath.Join(dstDir, "zarf.yaml"), "zarf.yaml layer should be staged before LoadFromDir is called")
}

func TestStagePackageLayer_CopiesWritableImagesIndex(t *testing.T) {
	src := filepath.Join(t.TempDir(), "index.json")
	require.NoError(t, os.WriteFile(src, []byte("original"), 0o400))
	dst := filepath.Join(t.TempDir(), "images", "index.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o700))

	require.NoError(t, stagePackageLayer(t.Context(), src, dst, "images/index.json"))
	require.NoError(t, os.WriteFile(dst, []byte("updated"), 0o600))

	sourceData, err := os.ReadFile(src)
	require.NoError(t, err)
	assert.Equal(t, "original", string(sourceData))
}

func TestExtractedArtifactPackageLayoutLoader_RejectsEscapingLayerTitle(t *testing.T) {
	const escapingTitle = "../../../escaped-zarf.yaml"
	loader := newArtifactPackageLayoutLoader(t, escapingTitle)
	dstDir := t.TempDir()
	escapedPath := filepath.Clean(filepath.Join(dstDir, filepath.FromSlash(escapingTitle)))

	_, err := loader.LoadPackageLayout(t.Context(), &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"}, dstDir, LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes destination directory")
	assert.NoFileExists(t, escapedPath)
}

func TestExtractedArtifactPackageLayoutLoader_RejectsMissingLayerTitle(t *testing.T) {
	loader := newArtifactPackageLayoutLoader(t, "")
	dstDir := t.TempDir()

	_, err := loader.LoadPackageLayout(t.Context(), &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"}, dstDir, LoadOptions{})
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
		_, err := loader.LoadPackageLayout(t.Context(), pkg, dstDir, LoadOptions{})
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
		_, err := loader.LoadPackageLayout(t.Context(), pkg, t.TempDir(), LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found in bundle artifact index")
	})
}

func TestExtractedArtifactPackageLayoutLoader_LoadBundle(t *testing.T) {
	validHCL := `uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test-bundle" }
package "mypkg" { source = "oci://example.com/pkg:v1" }
`
	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, bundlehcl.BundleFileName), []byte(validHCL), 0o600))
		loader := &ExtractedArtifactPackageLayoutLoader{}
		b, err := loader.LoadBundle(t.Context(), dir, LoadOptions{})
		require.NoError(t, err)
		assert.Equal(t, "test-bundle", b.Metadata.Name)
		require.Len(t, b.Packages, 1)
		assert.Equal(t, "mypkg", b.Packages[0].Name)
		assert.Equal(t, "oci://example.com/pkg:v1", b.Packages[0].Source)
	})

	t.Run("empty bundleDir", func(t *testing.T) {
		loader := &ExtractedArtifactPackageLayoutLoader{}
		_, err := loader.LoadBundle(t.Context(), "", LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bundleDir must not be empty")
	})

	t.Run("missing bundle file", func(t *testing.T) {
		loader := &ExtractedArtifactPackageLayoutLoader{}
		_, err := loader.LoadBundle(t.Context(), t.TempDir(), LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), bundlehcl.BundleFileName)
	})
}

// newArtifactPackageLayoutLoader creates an extracted-artifact loader fixture.
func newArtifactPackageLayoutLoader(t *testing.T, layerTitle string) *ExtractedArtifactPackageLayoutLoader {
	t.Helper()

	ociDir := t.TempDir()
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	require.NoError(t, os.MkdirAll(blobDir, 0o700))

	layerData := []byte("metadata:\n  name: mypkg\n")
	layerDigest := digest.FromBytes(layerData)
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, layerDigest.Encoded()), layerData, 0o600))

	manifest := ocispec.Manifest{Layers: []ocispec.Descriptor{{
		Digest:      layerDigest,
		Annotations: map[string]string{ocispec.AnnotationTitle: layerTitle},
	}}}
	manifestData, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDigest := digest.FromBytes(manifestData)
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, manifestDigest.Encoded()), manifestData, 0o600))

	return &ExtractedArtifactPackageLayoutLoader{
		OCIDir:         ociDir,
		PackageDigests: map[string]string{"example.com/pkg:v1": manifestDigest.String()},
	}
}

func TestExtractedArtifactPackageLayoutLoader_LoadPackage(t *testing.T) {
	loader := &ExtractedArtifactPackageLayoutLoader{}

	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte("metadata:\n  name: my-zarf-pkg\n"), 0o600))
		pkg, err := loader.LoadPackage(t.Context(), dir, LoadOptions{})
		require.NoError(t, err)
		assert.Equal(t, "my-zarf-pkg", pkg.Name)
		assert.Equal(t, dir, pkg.Source)
	})

	t.Run("empty packageDir", func(t *testing.T) {
		_, err := loader.LoadPackage(t.Context(), "", LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "packageDir must not be empty")
	})

	t.Run("missing zarf.yaml", func(t *testing.T) {
		_, err := loader.LoadPackage(t.Context(), t.TempDir(), LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading zarf.yaml")
	})

	t.Run("empty name in zarf.yaml", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte("metadata:\n  name: \"\"\n"), 0o600))
		_, err := loader.LoadPackage(t.Context(), dir, LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty metadata.name")
	})
}
