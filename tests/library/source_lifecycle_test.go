// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareSourcePreservesCallerFiles(t *testing.T) {
	for _, withDefaults := range []bool{false, true} {
		for _, directory := range []bool{false, true} {
			name := "file"
			if directory {
				name = "directory"
			}
			if withDefaults {
				name += " with defaults"
			}
			t.Run(name, func(t *testing.T) {
				fixture := createLibraryBundle(t)
				root, bundlePath, workspace := fixture.Root, fixture.BundleFile, t.TempDir()
				var defaultsPath string
				if withDefaults {
					defaultsPath = filepath.Join(root, "defaults.uds.hcl")
				} else {
					require.NoError(t, os.Remove(filepath.Join(root, "defaults.uds.hcl")))
				}
				input := bundlePath
				if directory {
					input = root
				}
				source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, input, workspace, runtime.GOARCH)
				require.NoError(t, err)
				t.Cleanup(func() { assert.NoError(t, source.Close()) })
				assert.Equal(t, bundlePath, source.BundlePath)
				assert.True(t, filepath.IsAbs(source.BundlePath))
				assert.Equal(t, defaultsPath, source.DefaultsPath)
				assert.Nil(t, source.Bundle)
				assert.Nil(t, source.Loader)
				require.NoError(t, source.Close())
				require.NoError(t, source.Close())
				assert.FileExists(t, bundlePath)
				if withDefaults {
					assert.FileExists(t, defaultsPath)
				}
				entries, err := os.ReadDir(workspace)
				require.NoError(t, err)
				assert.Empty(t, entries)
			})
		}
	}
}

func TestPrepareRelativeSourceReturnsAbsolutePaths(t *testing.T) {
	fixture := createLibraryBundle(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	for _, input := range []string{fixture.Root, fixture.BundleFile} {
		relative, err := filepath.Rel(cwd, input)
		require.NoError(t, err)
		source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, relative, t.TempDir(), runtime.GOARCH)
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, source.Close()) })
		assert.Equal(t, fixture.BundleFile, source.BundlePath)
		assert.Equal(t, filepath.Join(fixture.Root, "defaults.uds.hcl"), source.DefaultsPath)
		assert.True(t, filepath.IsAbs(source.BundlePath))
		assert.True(t, filepath.IsAbs(source.DefaultsPath))
	}
}

func TestPrepareArtifactOwnsExtractedResources(t *testing.T) {
	artifact := createLibraryArtifact(t)
	workspace := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	relativeWorkspace, err := filepath.Rel(cwd, workspace)
	require.NoError(t, err)
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, artifact, relativeWorkspace, runtime.GOARCH)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, source.Close()) })
	assert.True(t, filepath.IsAbs(source.BundlePath))
	assert.FileExists(t, source.BundlePath)
	require.NotNil(t, source.Bundle)
	assert.Equal(t, "library-fixture", source.Bundle.Metadata.Name)
	require.NotNil(t, source.Loader)
	assert.FileExists(t, source.DefaultsPath)
	assert.True(t, filepath.IsAbs(source.DefaultsPath))
	defaults, err := os.ReadFile(source.DefaultsPath)
	require.NoError(t, err)
	assert.Contains(t, string(defaults), `fixture = "default"`)
	require.Len(t, source.Bundle.Packages, 2)
	require.Len(t, source.Bundle.Packages[1].ValuesFiles, 1)
	valuesPath := filepath.Join(filepath.Dir(source.BundlePath), source.Bundle.Packages[1].ValuesFiles[0])
	values, err := os.ReadFile(valuesPath)
	require.NoError(t, err)
	assert.Equal(t, "replicas: 1\n", string(values))

	loaded, err := source.Loader.LoadPackageLayout(t.Context(), &source.Bundle.Packages[0], t.TempDir(), bundle.ZarfPackageLayoutLoadOptions{})
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "base", loaded.Layout.PackageDefinition.AsV1alpha1().Metadata.Name)
	assert.True(t, loaded.IsPartial) // Bundle artifacts contain selected package layers.

	require.NoError(t, source.Close())
	require.NoError(t, source.Close())
	assert.NoFileExists(t, source.BundlePath)
	assert.NoFileExists(t, source.DefaultsPath)
	assert.NoFileExists(t, valuesPath)
	assert.FileExists(t, artifact)
	entries, err := os.ReadDir(workspace)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestPrepareArtifactFailureCleansWorkspace(t *testing.T) {
	artifact := filepath.Join(t.TempDir(), "invalid.tar.zst")
	require.NoError(t, os.WriteFile(artifact, []byte("invalid archive"), 0o600))
	workspace := t.TempDir()
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, artifact, workspace, runtime.GOARCH)
	require.ErrorIs(t, err, bundle.ErrPrepareDeploySource)
	assert.Nil(t, source)
	entries, err := os.ReadDir(workspace)
	require.NoError(t, err)
	assert.Empty(t, entries)
	assert.FileExists(t, artifact)
}

func TestPrepareArtifactResolvesDefaultTemporaryDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TMPDIR configures the default temporary directory on Unix")
	}
	artifact, workspace := createLibraryArtifact(t), t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	relativeWorkspace, err := filepath.Rel(cwd, workspace)
	require.NoError(t, err)
	t.Setenv("TMPDIR", relativeWorkspace)
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, artifact, "", runtime.GOARCH)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, source.Close()) })
	assert.True(t, filepath.IsAbs(source.BundlePath))
	assert.True(t, filepath.IsAbs(source.DefaultsPath))
	assert.FileExists(t, source.BundlePath)
	require.NoError(t, source.Close())
	entries, err := os.ReadDir(workspace)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestDeploySourceCloseAcceptsEmptySource(t *testing.T) {
	var source *bundle.DeploySource
	require.NoError(t, source.Close())
	source = &bundle.DeploySource{}
	require.NoError(t, source.Close())
	require.NoError(t, source.Close())
}
