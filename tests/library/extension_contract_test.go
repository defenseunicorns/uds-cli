// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contractLoader struct {
	root string
	load func(context.Context, *spec.Package, string, bundle.ZarfPackageLayoutLoadOptions) (*bundle.ZarfPackageLayoutLoadResult, error)
}

var (
	_ bundle.ZarfPackageLayoutLoader    = (*contractLoader)(nil)
	_ bundle.PackageStagingRootProvider = (*contractLoader)(nil)
)

func (l *contractLoader) PackageStagingRoot(context.Context) string { return l.root }

func (l *contractLoader) LoadPackageLayout(ctx context.Context, pkg *spec.Package, dst string, opts bundle.ZarfPackageLayoutLoadOptions) (*bundle.ZarfPackageLayoutLoadResult, error) {
	return l.load(ctx, pkg, dst, opts)
}

func preparedLibrarySource(t *testing.T) *bundle.DeploySource {
	t.Helper()
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, createLibraryArtifact(t), t.TempDir(), runtime.GOARCH)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, source.Close()) })
	return source
}

func TestLayoutDeployedDigestContract(t *testing.T) {
	var empty *bundle.ZarfPackageLayout
	empty.SetDeployedDigest("ignored")
	layout := &bundle.ZarfPackageLayout{}
	assert.Empty(t, layout.Digest())
	layout.SetDeployedDigest("sha256:first")
	assert.Equal(t, "sha256:first", layout.Digest())
	layout.SetDeployedDigest("sha256:replacement")
	assert.Equal(t, "sha256:replacement", layout.Digest())
}

func TestCustomLoaderStagingAndCleanup(t *testing.T) {
	for _, mode := range []string{"preferred", "empty", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			source := preparedLibrarySource(t)
			delegate := source.Loader
			config := libraryFixtureConfig(t)
			root, expectedRoot := "", config.Options.TmpDir
			switch mode {
			case "preferred":
				root = t.TempDir()
				expectedRoot = root
			case "unavailable":
				root = filepath.Join(t.TempDir(), "missing", "staging")
			}
			var stagedPath, loadedName, hookPath, hookDigest, hookName string
			var loadPartial, hookPartial bool
			streams, _, output, _ := iostreams.NewTestIOStreams()
			source.Loader = &contractLoader{root: root, load: func(ctx context.Context, pkg *spec.Package, dst string, opts bundle.ZarfPackageLayoutLoadOptions) (*bundle.ZarfPackageLayoutLoadResult, error) {
				stagedPath, loadedName, loadPartial = dst, pkg.Name, opts.IsPartial
				if _, err := fmt.Fprint(opts.Streams.Out(), "consumer loader output"); err != nil {
					return nil, err
				}
				result, err := delegate.LoadPackageLayout(ctx, pkg, dst, opts)
				if err != nil {
					return nil, err
				}
				result.IsPartial = true
				result.Layout.SetDeployedDigest("sha256:consumer-resolved")
				return result, nil
			}}
			stop := errors.New("contract observed before cluster deployment")
			result, err := bundle.Deploy(t.Context(), source, bundle.DeployOptions{
				Config: config, Streams: streams,
				PackageDeployHooks: bundle.PackageDeployHooks{PreDeploy: func(_ context.Context, _ *spec.Package, layout *bundle.ZarfPackageLayout, opts *bundle.DeployPackageOptions) error {
					hookPath, hookDigest, hookPartial = layout.DirPath(), layout.Digest(), opts.IsPartial
					hookName = layout.PackageDefinition.AsV1alpha1().Metadata.Name
					return stop
				}},
			})
			require.ErrorIs(t, err, stop)
			require.ErrorIs(t, err, bundle.ErrDeployBundle)
			assert.Nil(t, result)
			assert.Equal(t, "base", loadedName)
			assert.Equal(t, "base", hookName)
			assert.False(t, loadPartial)
			assert.True(t, hookPartial)
			assert.Equal(t, "consumer loader output", output.String())
			assert.Equal(t, "sha256:consumer-resolved", hookDigest)
			assert.Equal(t, stagedPath, hookPath)
			assert.True(t, filepath.IsAbs(hookPath))
			// Compare directory identity: macOS may canonicalize /var to /private/var.
			want, err := os.Stat(expectedRoot)
			require.NoError(t, err)
			got, err := os.Stat(filepath.Dir(stagedPath))
			require.NoError(t, err)
			assert.True(t, os.SameFile(want, got))
			assert.NoDirExists(t, stagedPath)
			assert.FileExists(t, source.BundlePath)
		})
	}
}

func TestCustomLoaderErrorPreservesIdentityAndCleansStaging(t *testing.T) {
	source := preparedLibrarySource(t)
	loadErr := errors.New("consumer loader failed")
	var stagedPath string
	source.Loader = &contractLoader{root: t.TempDir(), load: func(_ context.Context, _ *spec.Package, dst string, _ bundle.ZarfPackageLayoutLoadOptions) (*bundle.ZarfPackageLayoutLoadResult, error) {
		stagedPath = dst
		return nil, loadErr
	}}
	called := false
	result, err := bundle.Deploy(t.Context(), source, bundle.DeployOptions{
		Config: libraryFixtureConfig(t),
		PackageDeployHooks: bundle.PackageDeployHooks{PreDeploy: func(context.Context, *spec.Package, *bundle.ZarfPackageLayout, *bundle.DeployPackageOptions) error {
			called = true
			return nil
		}},
	})
	require.ErrorIs(t, err, loadErr)
	require.ErrorIs(t, err, bundle.ErrDeployBundle)
	assert.Nil(t, result)
	assert.False(t, called)
	require.NotEmpty(t, stagedPath)
	assert.NoDirExists(t, stagedPath)
}
