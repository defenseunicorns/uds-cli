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

	"github.com/defenseunicorns/uds-cli/internal/logger"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
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
			name:         "OCI source key uses ref.name",
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
			name:         "missing OCI ref name",
			pkg:          &Package{Name: "other", Source: "oci://example.com/other:v1"},
			digests:      map[string]string{"example.com/pkg:v1": "sha256:aaa"},
			wantNotFound: true,
		},
		{
			name:         "missing local source is rejected",
			pkg:          &Package{Name: "absent", Source: "./absent"},
			digests:      map[string]string{"localpkg": "sha256:bbb"},
			wantNotFound: true,
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
			_, _, err := loader.LoadPackageLayout(t.Context(), tt.pkg, t.TempDir(), LoadOptions{})
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

func TestExtractedArtifactPackageLayoutLoaderPackageStagingRoot(t *testing.T) {
	tests := []struct {
		name   string
		ociDir string
		want   string
	}{
		{name: "empty OCI directory", want: ""},
		{name: "OCI directory", ociDir: "/workspace/oci", want: "/workspace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := &ExtractedArtifactPackageLayoutLoader{OCIDir: tt.ociDir}
			assert.Equal(t, tt.want, loader.PackageStagingRoot(t.Context()))
		})
	}
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

			pkgLayout, _, err := loader.LoadPackageLayout(t.Context(), &Package{
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

	// LoadPackageLayout fails at layout.LoadFromDir since fixture has fake content,
	// not a valid Zarf package. Confirm layer files were staged before that failure.
	_, _, err := loader.LoadPackageLayout(t.Context(), pkg, dstDir, LoadOptions{})
	require.Error(t, err)

	assert.FileExists(t, filepath.Join(dstDir, "zarf.yaml"), "zarf.yaml layer should be staged before LoadFromDir is called")
}

func TestExtractedArtifactPackageLayoutLoader_StagesFilesWithRelativeOCIDir(t *testing.T) {
	loader := newArtifactPackageLayoutLoader(t, "zarf.yaml")
	workspaceDir := filepath.Dir(loader.OCIDir)
	t.Chdir(workspaceDir)
	loader.OCIDir = filepath.Base(loader.OCIDir)

	dstDir := t.TempDir()
	_, _, err := loader.LoadPackageLayout(t.Context(), &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"}, dstDir, LoadOptions{})
	require.Error(t, err, "fixture is not a complete Zarf package")
	assert.FileExists(t, filepath.Join(dstDir, "zarf.yaml"))
}

func TestStageArtifactPackageLayer_CopiesFile(t *testing.T) {
	workspaceDir := t.TempDir()
	src := filepath.Join(workspaceDir, "index.json")
	require.NoError(t, os.WriteFile(src, []byte("original"), 0o400))
	root, err := os.OpenRoot(workspaceDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	require.NoError(t, root.MkdirAll("images", 0o700))

	require.NoError(t, stageArtifactPackageLayer(t.Context(), root, "index.json", root, "images/index.json", "images/index.json", true))
	require.NoError(t, os.WriteFile(filepath.Join(workspaceDir, "images", "index.json"), []byte("updated"), 0o600))

	sourceData, err := os.ReadFile(src)
	require.NoError(t, err)
	assert.Equal(t, "original", string(sourceData))
}

func TestStageArtifactPackageLayer_HardLinksRegularBlob(t *testing.T) {
	workspaceDir := t.TempDir()
	src := filepath.Join(workspaceDir, "blob")
	require.NoError(t, os.WriteFile(src, []byte("contents"), 0o400))
	root, err := os.OpenRoot(workspaceDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })

	require.NoError(t, stageArtifactPackageLayer(t.Context(), root, "blob", root, "staged", "zarf.yaml", true))
	srcInfo, err := os.Stat(src)
	require.NoError(t, err)
	dstInfo, err := root.Stat("staged")
	require.NoError(t, err)
	assert.True(t, os.SameFile(srcInfo, dstInfo), "regular blob should be hard-linked into staging")
}

func TestStageArtifactPackageLayer_CopiesSymlinkedBlob(t *testing.T) {
	workspaceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workspaceDir, "blob"), []byte("contents"), 0o600))
	require.NoError(t, os.Symlink("blob", filepath.Join(workspaceDir, "blob-link")))
	root, err := os.OpenRoot(workspaceDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })

	require.NoError(t, stageArtifactPackageLayer(t.Context(), root, "blob-link", root, "staged", "zarf.yaml", true))
	info, err := root.Lstat("staged")
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink)
	data, err := root.ReadFile("staged")
	require.NoError(t, err)
	assert.Equal(t, "contents", string(data))
}

func TestCopyFileContentsBetweenRoots_PreservesExistingTmpPath(t *testing.T) {
	workspaceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workspaceDir, "source"), []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workspaceDir, "target.tmp"), []byte("existing"), 0o600))
	root, err := os.OpenRoot(workspaceDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })

	require.NoError(t, copyFileContentsBetweenRoots(t.Context(), root, "source", root, "target"))
	tmpData, err := root.ReadFile("target.tmp")
	require.NoError(t, err)
	assert.Equal(t, "existing", string(tmpData))
}

func TestExtractedArtifactPackageLayoutLoader_RejectsEscapingLayerTitle(t *testing.T) {
	const escapingTitle = "../../../escaped-zarf.yaml"
	loader := newArtifactPackageLayoutLoader(t, escapingTitle)
	dstDir := t.TempDir()
	escapedPath := filepath.Clean(filepath.Join(dstDir, filepath.FromSlash(escapingTitle)))

	_, _, err := loader.LoadPackageLayout(t.Context(), &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"}, dstDir, LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes destination directory")
	assert.NoFileExists(t, escapedPath)
}

func TestExtractedArtifactPackageLayoutLoader_RejectsDestinationSymlinkEscape(t *testing.T) {
	loader := newArtifactPackageLayoutLoader(t, "escape/zarf.yaml")
	dstDir := t.TempDir()
	escapedDir := t.TempDir()
	require.NoError(t, os.Symlink(escapedDir, filepath.Join(dstDir, "escape")))

	_, _, err := loader.LoadPackageLayout(t.Context(), &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"}, dstDir, LoadOptions{})
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(escapedDir, "zarf.yaml"))
}

func TestExtractedArtifactPackageLayoutLoader_RejectsMissingLayerTitle(t *testing.T) {
	loader := newArtifactPackageLayoutLoader(t, "")
	dstDir := t.TempDir()

	_, _, err := loader.LoadPackageLayout(t.Context(), &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"}, dstDir, LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `manifest for package "mypkg" missing title annotation on layer`)

	entries, readErr := os.ReadDir(dstDir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "layer without title should not be staged")
}

func TestCopyFileContents_ObservesCanceledContext(t *testing.T) {
	workspaceDir := t.TempDir()
	src := filepath.Join(workspaceDir, "src")
	root, err := os.OpenRoot(workspaceDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	require.NoError(t, os.WriteFile(src, []byte("contents"), 0o600))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = copyFileContentsBetweenRoots(ctx, root, "src", root, "dst")
	require.ErrorIs(t, err, context.Canceled)
	assert.NoFileExists(t, filepath.Join(workspaceDir, "dst"))
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

func TestExtractedArtifactPackageLayoutLoader_RejectsUnindexedLocalSource(t *testing.T) {
	t.Run("does not read or stage files from pkg.Source", func(t *testing.T) {
		pkgDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "zarf.yaml"), []byte("kind: ZarfPackageConfig\nmetadata:\n  name: test\n"), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, "components"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "components", "test.tar"), []byte("dummy"), 0o600))

		dstDir := t.TempDir()
		loader := &ExtractedArtifactPackageLayoutLoader{PackageDigests: map[string]string{}}
		pkg := &Package{Name: "mypkg", Source: pkgDir}

		_, _, err := loader.LoadPackageLayout(t.Context(), pkg, dstDir, LoadOptions{})
		require.Error(t, err)

		assert.Contains(t, err.Error(), "not found in bundle artifact index")
		assert.NoFileExists(t, filepath.Join(dstDir, "zarf.yaml"))
		assert.NoDirExists(t, filepath.Join(dstDir, "components"))

		// Source directory is preserved.
		assert.DirExists(t, pkgDir, "source dir should be preserved after staging")
		assert.FileExists(t, filepath.Join(pkgDir, "zarf.yaml"), "original zarf.yaml should be preserved")
		assert.FileExists(t, filepath.Join(pkgDir, "components", "test.tar"), "original test.tar should be preserved")
	})

	t.Run("OCI source not in PackageDigests returns error (not dir fallback)", func(t *testing.T) {
		loader := &ExtractedArtifactPackageLayoutLoader{PackageDigests: map[string]string{}}
		pkg := &Package{Name: "mypkg", Source: "oci://example.com/pkg:v1"}
		_, _, err := loader.LoadPackageLayout(t.Context(), pkg, t.TempDir(), LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found in bundle artifact index")
	})
}

// newArtifactPackageLayoutLoader creates an extracted-artifact loader fixture.
func newArtifactPackageLayoutLoader(t *testing.T, layerTitle string) *ExtractedArtifactPackageLayoutLoader {
	t.Helper()

	ociDir := t.TempDir()
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	_, err := udsoci.CreateStore(ociDir)
	require.NoError(t, err)

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
		OCIDir: ociDir,
		PackageManifests: map[string]ocispec.Descriptor{"example.com/pkg:v1": {
			MediaType: ocispec.MediaTypeImageManifest,
			Digest:    manifestDigest,
			Size:      int64(len(manifestData)),
		}},
	}
}
