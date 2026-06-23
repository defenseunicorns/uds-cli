// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractedArtifactPackageLayoutLoader_LoadBundle(t *testing.T) {
	validHCL := `uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test-bundle" }
package "mypkg" { source = "oci://example.com/pkg:v1" }
`
	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, BundleFileName), []byte(validHCL), 0o600))
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
		assert.Contains(t, err.Error(), BundleFileName)
	})
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
