// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareDeploySource_Directory(t *testing.T) {
	t.Run("directory containing bundle.uds.hcl", func(t *testing.T) {
		dir := t.TempDir()
		bundleFile := filepath.Join(dir, BundleFileName)
		require.NoError(t, os.WriteFile(bundleFile, []byte(""), 0o600))

		src, err := PrepareDeploySource(t.Context(), iostreams.IOStreams{}, dir, "")
		require.NoError(t, err)
		defer func() { require.NoError(t, src.Close()) }()

		assert.Equal(t, bundleFile, src.BundlePath)
		assert.Nil(t, src.Loader)
		assert.Nil(t, src.ValuesFilesOverride)
	})

	t.Run("direct bundle.uds.hcl path", func(t *testing.T) {
		dir := t.TempDir()
		bundleFile := filepath.Join(dir, BundleFileName)
		require.NoError(t, os.WriteFile(bundleFile, []byte(""), 0o600))

		src, err := PrepareDeploySource(t.Context(), iostreams.IOStreams{}, bundleFile, "")
		require.NoError(t, err)
		defer func() { require.NoError(t, src.Close()) }()

		assert.Equal(t, bundleFile, src.BundlePath)
		assert.Nil(t, src.Loader)
		assert.Nil(t, src.ValuesFilesOverride)
	})

	t.Run("tar.zst suffix always uses artifact preparation", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "source.tar.zst")
		require.NoError(t, os.Mkdir(dir, 0o700))
		bundleFile := filepath.Join(dir, BundleFileName)
		require.NoError(t, os.WriteFile(bundleFile, []byte(""), 0o600))

		_, err := PrepareDeploySource(t.Context(), iostreams.IOStreams{}, dir, "")
		require.ErrorContains(t, err, "extracting bundle artifact")
	})

	t.Run("cleanup is safe to call", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, BundleFileName), []byte(""), 0o600))

		src, err := PrepareDeploySource(t.Context(), iostreams.IOStreams{}, dir, "")
		require.NoError(t, err)

		// Directory sources do not own resources; Close must be a no-op.
		require.NoError(t, src.Close())
	})
}

func TestPrepareDeploySource_TarZst(t *testing.T) {
	bundleHCL := `uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test-bundle" version = "0.1.0" }
package "mypkg" {
  source = "oci://example.com/pkg:v1"
  values_files = ["original-0.yaml", "original-1.yaml"]
}
`
	tarPath := buildBundleArtifact(t, bundleHCL, map[string][]string{
		"mypkg": {"key: value1", "key: value2"},
	}, []string{"example.com/pkg:v1"})
	tmpRoot := t.TempDir()

	src, err := PrepareDeploySource(t.Context(), iostreams.IOStreams{}, tarPath, tmpRoot)
	require.NoError(t, err)
	require.NotNil(t, src)

	workspaceDir := filepath.Dir(src.BundlePath)
	assert.Equal(t, BundleFileName, filepath.Base(src.BundlePath))
	assert.Equal(t, tmpRoot, filepath.Dir(workspaceDir))

	loader, ok := src.Loader.(*ExtractedArtifactPackageLayoutLoader)
	require.True(t, ok)
	assert.Equal(t, filepath.Join(workspaceDir, "oci"), loader.OCIDir)
	assert.Contains(t, loader.PackageDigests, "example.com/pkg:v1")

	require.Contains(t, src.ValuesFilesOverride, "mypkg")
	require.Len(t, src.ValuesFilesOverride["mypkg"], 2)
	assert.Equal(t, filepath.Join(workspaceDir, "values", "mypkg", "0.yaml"), src.ValuesFilesOverride["mypkg"][0])
	assert.Equal(t, filepath.Join(workspaceDir, "values", "mypkg", "1.yaml"), src.ValuesFilesOverride["mypkg"][1])
	for _, path := range src.ValuesFilesOverride["mypkg"] {
		_, err := os.Stat(path)
		require.NoError(t, err)
	}

	require.NoError(t, src.Close())
	_, err = os.Stat(workspaceDir)
	assert.True(t, os.IsNotExist(err), "artifact workspace should be removed on Close")
}
