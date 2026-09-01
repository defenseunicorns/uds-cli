// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/filesystem"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/mholt/archives"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

func TestIsPackageArchive(t *testing.T) {
	t.Parallel()
	assert.True(t, isPackageArchive("package.tar.zst"))
	assert.True(t, isPackageArchive("package.tar"))
	assert.True(t, isPackageArchive("package.tar.zst.part000"))
	assert.False(t, isPackageArchive("package.tar.zst.part001"))
	assert.False(t, isPackageArchive("package.zip"))
}

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func writeMinimalZarfPackage(t *testing.T, dir, name string) {
	t.Helper()
	zarfYAML := "kind: ZarfPackageConfig\nmetadata:\n  name: " + name + "\n  version: 1.0.0\n  architecture: amd64\n  aggregateChecksum: " + emptySHA256 + "\nbuild:\n  architecture: amd64\n  version: test\ncomponents: []\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, layout.ZarfYAML), []byte(zarfYAML), filesystem.PrivateFileMode))
	require.NoError(t, os.WriteFile(filepath.Join(dir, layout.Checksums), nil, filesystem.PrivateFileMode))
}

func newTestStore(t *testing.T) *udsoci.Store {
	t.Helper()
	store, err := udsoci.CreateStore(t.TempDir())
	require.NoError(t, err)
	return store
}

func TestLocalSourceIngestFilteredUsesCanonicalPackageLayout(t *testing.T) {
	pkgDir := t.TempDir()
	writeMinimalZarfPackage(t, pkgDir, "test")
	store := newTestStore(t)

	source := &localSource{path: pkgDir, arch: "amd64", streams: iostreams.IOStreams{}}
	descriptors, err := source.IngestFiltered(t.Context(), filters.Empty(), store)
	require.NoError(t, err)
	require.Len(t, descriptors, 1)

	data, err := udsoci.FetchBytes(t.Context(), store, descriptors[0])
	require.NoError(t, err)
	assert.Contains(t, string(data), layout.ZarfYAML)
	assert.Equal(t, "application/vnd.oci.image.manifest.v1+json", descriptors[0].MediaType)
}

func TestLocalSourceRejectsOCILayoutDirectory(t *testing.T) {
	layoutDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(layoutDir, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), filesystem.PrivateFileMode))

	source := &localSource{path: layoutDir, arch: "amd64", streams: iostreams.IOStreams{}}
	_, err := source.IngestFiltered(t.Context(), filters.Empty(), newTestStore(t))
	require.ErrorContains(t, err, "not a Zarf package directory")
}

func TestLocalSourceRejectsSymlinkInPackageDirectory(t *testing.T) {
	pkgDir := t.TempDir()
	writeMinimalZarfPackage(t, pkgDir, "symlink")
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.WriteFile(target, []byte("target"), filesystem.PrivateFileMode))
	symlinkPath := filepath.Join(pkgDir, "linked-file")
	if err := os.Symlink(target, symlinkPath); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	source := &localSource{path: pkgDir, arch: "amd64", streams: iostreams.IOStreams{}}
	_, err := source.IngestFiltered(t.Context(), filters.Empty(), newTestStore(t))
	require.ErrorContains(t, err, "unsupported symlink")
}

func TestLocalSourceIngestFilteredRelativePath(t *testing.T) {
	bundleDir := t.TempDir()
	pkgDir := filepath.Join(bundleDir, "my-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, filesystem.PrivateDirectoryMode))
	writeMinimalZarfPackage(t, pkgDir, "relative")

	source := &localSource{path: "my-pkg", arch: "amd64", bundleDir: bundleDir}
	descriptors, err := source.IngestFiltered(t.Context(), filters.Empty(), newTestStore(t))
	require.NoError(t, err)
	assert.Len(t, descriptors, 1)
}

func TestLocalSourcePullFilteredArchiveCleansWorkspaceOnError(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "zarf-package-bad-amd64-1.0.0.tar.zst")
	require.NoError(t, os.WriteFile(archivePath, []byte("not a zstd archive"), filesystem.PrivateFileMode))
	workspace := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, os.MkdirAll(workspace, filesystem.PrivateDirectoryMode))

	source := &localSource{path: archivePath, arch: "amd64", streams: iostreams.IOStreams{}}
	_, err := source.PullFiltered(t.Context(), workspace, layout.PackageLayoutOptions{
		Filter:               filters.Empty(),
		VerificationStrategy: layout.VerifyNever,
	})

	require.Error(t, err)
	entries, readErr := os.ReadDir(workspace)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestLocalSourcePullFilteredArchiveUsesZarfLoader(t *testing.T) {
	pkgDir := t.TempDir()
	writeMinimalZarfPackage(t, pkgDir, "archive")
	archivePath := filepath.Join(t.TempDir(), "zarf-package-archive-amd64-1.0.0.tar.zst")
	require.NoError(t, writeTestTarZst(t, archivePath, pkgDir))

	workspace := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, os.MkdirAll(workspace, filesystem.PrivateDirectoryMode))
	source := &localSource{path: archivePath, arch: "amd64", streams: iostreams.IOStreams{}}
	pkgLayout, err := source.PullFiltered(t.Context(), workspace, layout.PackageLayoutOptions{
		Filter:               filters.Empty(),
		VerificationStrategy: layout.VerifyNever,
	})
	require.NoError(t, err)
	assert.Contains(t, pkgLayout.DirPath(), workspace)
	require.NoError(t, pkgLayout.Cleanup())
}

func TestLocalSourcePullFilteredUncompressedArchiveUsesZarfLoader(t *testing.T) {
	pkgDir := t.TempDir()
	writeMinimalZarfPackage(t, pkgDir, "archive")
	archivePath := filepath.Join(t.TempDir(), "zarf-package-archive-amd64-1.0.0.tar")
	require.NoError(t, writeTestTar(t, archivePath, pkgDir))

	workspace := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, os.MkdirAll(workspace, filesystem.PrivateDirectoryMode))
	source := &localSource{path: archivePath, arch: "amd64", streams: iostreams.IOStreams{}}
	pkgLayout, err := source.PullFiltered(t.Context(), workspace, layout.PackageLayoutOptions{
		Filter:               filters.Empty(),
		VerificationStrategy: layout.VerifyNever,
	})
	require.NoError(t, err)
	assert.Contains(t, pkgLayout.DirPath(), workspace)
	require.NoError(t, pkgLayout.Cleanup())
}

func TestLocalSourcePullFilteredSplitArchivePreservesSourceParts(t *testing.T) {
	pkgDir := t.TempDir()
	writeMinimalZarfPackage(t, pkgDir, "split")
	archivePath := filepath.Join(t.TempDir(), "zarf-package-split-amd64-1.0.0.tar.zst")
	require.NoError(t, writeTestTarZst(t, archivePath, pkgDir))
	archive, err := os.ReadFile(archivePath)
	require.NoError(t, err)
	part000 := archivePath + ".part000"
	part001 := archivePath + ".part001"
	require.NoError(t, os.WriteFile(part000, []byte(`{"Count":1}`), filesystem.PrivateFileMode))
	// part001 is a test-owned path beneath t.TempDir.
	//nolint:gosec // G703 reports the derived archive path as tainted.
	require.NoError(t, os.WriteFile(part001, archive, filesystem.PrivateFileMode))
	require.NoError(t, os.Remove(archivePath))

	workspace := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, os.MkdirAll(workspace, filesystem.PrivateDirectoryMode))
	source := &localSource{path: part000, arch: "amd64", streams: iostreams.IOStreams{}}
	pkgLayout, err := source.PullFiltered(t.Context(), workspace, layout.PackageLayoutOptions{
		Filter:               filters.Empty(),
		VerificationStrategy: layout.VerifyNever,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
	assert.FileExists(t, part000)
	assert.FileExists(t, part001)
	assert.Equal(t, "split", pkgLayout.AsV1alpha1().Metadata.Name)
}

func writeTestTarZst(t *testing.T, archivePath, srcDir string) error {
	t.Helper()
	files, err := archives.FilesFromDisk(t.Context(), nil, map[string]string{srcDir + string(filepath.Separator): ""})
	if err != nil {
		return err
	}
	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	archive := archives.CompressedArchive{Archival: archives.Tar{}, Compression: archives.Zstd{}}
	if err := archive.Archive(t.Context(), f, files); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeTestTar(t *testing.T, archivePath, srcDir string) error {
	t.Helper()
	files, err := archives.FilesFromDisk(t.Context(), nil, map[string]string{srcDir + string(filepath.Separator): ""})
	if err != nil {
		return err
	}
	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	if err := (archives.Tar{}).Archive(t.Context(), f, files); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func TestLocalSourceResolvedPath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		bundleDir string
		want      string
	}{
		{name: "absolute", path: "/abs/path/pkg", bundleDir: "/bundle", want: "/abs/path/pkg"},
		{name: "relative", path: "my-pkg", bundleDir: "/bundle/dir", want: "/bundle/dir/my-pkg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &localSource{path: tt.path, bundleDir: tt.bundleDir}
			assert.Equal(t, tt.want, source.resolvedPath())
		})
	}
}

func TestIsZarfPackage(t *testing.T) {
	root := t.TempDir()
	assert.False(t, isZarfPackage(root))
	require.NoError(t, os.WriteFile(filepath.Join(root, layout.ZarfYAML), []byte("metadata:\n  name: test"), filesystem.PrivateFileMode))
	assert.True(t, isZarfPackage(root))
}
