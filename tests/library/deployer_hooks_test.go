// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"context"
	"errors"
	"os"
	"runtime"
	"sync"
	"testing"

	bundle "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mkStaticLoader(pkg *bundle.ZarfPackageLayout, err error) bundle.ZarfPackageLayoutLoader {
	return &staticLoaderImpl{pkg: pkg, err: err}
}

type staticLoaderImpl struct {
	pkg *bundle.ZarfPackageLayout
	err error
}

func (l *staticLoaderImpl) LoadPackageLayout(_ context.Context, _ *spec.Package, _ string, _ bundle.ZarfPackageLayoutLoadOptions) (*bundle.ZarfPackageLayout, error) {
	return l.pkg, l.err
}

func imageLayoutLib() *bundle.ZarfPackageLayout {
	return &bundle.ZarfPackageLayout{
		Pkg: bundle.ZarfPackage{
			Components: []bundle.ZarfPackageComponent{
				{
					Name:   "main",
					Images: []string{"ghcr.io/example/image:v1"},
					ImageArchives: []bundle.ZarfPackageImageArchive{
						{Path: "archive.tar", Images: []string{"ghcr.io/example/image:v1"}},
					},
				},
			},
		},
	}
}

func singlePkgBundle() *spec.UDSBundle {
	return &spec.UDSBundle{
		UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: spec.Metadata{Name: "test-bundle"},
		Packages: []spec.Package{{Name: "alpha", Source: "oci://example.com/alpha:v1"}},
	}
}

func twoPkgLinearBundle() *spec.UDSBundle {
	return &spec.UDSBundle{
		UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: spec.Metadata{Name: "test-bundle"},
		Packages: []spec.Package{
			{Name: "alpha", Source: "oci://example.com/alpha:v1"},
			{Name: "beta", Source: "oci://example.com/beta:v1", DependsOn: []spec.PackageRef{{Name: "alpha"}}},
		},
	}
}

func newTestConfig() *bundle.UDSBundleConfig {
	return &bundle.UDSBundleConfig{
		Options: &bundle.ConfigOptions{Architecture: runtime.GOARCH, TmpDir: os.TempDir(), Concurrency: 10},
	}
}

func deploySource(b *spec.UDSBundle) *bundle.DeploySource {
	return &bundle.DeploySource{
		BundlePath: "/tmp/bundle/bundle.uds.hcl",
		Bundle:     b,
		Loader:     mkStaticLoader(imageLayoutLib(), nil),
	}
}

func TestPackageHooks_PreDeployImageSkip(t *testing.T) {
	hookErr := errors.New("stop after layout mutation")
	var captured *bundle.ZarfPackageLayout
	_, err := bundle.Deploy(t.Context(), deploySource(singlePkgBundle()), bundle.DeployOptions{
		Config: newTestConfig(),
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy: func(_ context.Context, _ *spec.Package, pkgLayout *bundle.ZarfPackageLayout, _ *bundle.DeployPackageOptions) error {
				captured = pkgLayout
				for i := range pkgLayout.Pkg.Components {
					pkgLayout.Pkg.Components[i].Images = nil
					pkgLayout.Pkg.Components[i].ImageArchives = nil
				}
				return hookErr
			},
		},
	})

	require.ErrorIs(t, err, hookErr)
	require.NotNil(t, captured)
	assert.Empty(t, captured.Pkg.Components[0].Images)
	assert.Empty(t, captured.Pkg.Components[0].ImageArchives)
}

func TestPackageHooks_PreDeployErrorAbortsBeforeClusterDeploy(t *testing.T) {
	hookErr := errors.New("pre-deploy hook failed")
	_, err := bundle.Deploy(t.Context(), deploySource(singlePkgBundle()), bundle.DeployOptions{
		Config: newTestConfig(),
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy: func(context.Context, *spec.Package, *bundle.ZarfPackageLayout, *bundle.DeployPackageOptions) error {
				return hookErr
			},
		},
	})

	require.ErrorIs(t, err, hookErr)
}

func TestBundleHooks_PreDeployRunsBeforePackageHook(t *testing.T) {
	var mu sync.Mutex
	var order []string
	record := func(value string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, value)
	}
	hookErr := errors.New("stop after package hook")
	_, err := bundle.Deploy(t.Context(), deploySource(twoPkgLinearBundle()), bundle.DeployOptions{
		Config: newTestConfig(),
		BundleDeployHooks: bundle.BundleDeployHooks{
			PreDeploy: func(context.Context, *spec.UDSBundle, *bundle.DeployOptions) error {
				record("bundle")
				return nil
			},
		},
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy: func(context.Context, *spec.Package, *bundle.ZarfPackageLayout, *bundle.DeployPackageOptions) error {
				record("package")
				return hookErr
			},
		},
	})

	require.ErrorIs(t, err, hookErr)
	assert.Equal(t, []string{"bundle", "package"}, order)
}

func TestBundleHooks_PreDeployCanInstallPackageHook(t *testing.T) {
	hookErr := errors.New("installed package hook")
	var invoked bool
	_, err := bundle.Deploy(t.Context(), deploySource(singlePkgBundle()), bundle.DeployOptions{
		Config: newTestConfig(),
		BundleDeployHooks: bundle.BundleDeployHooks{
			PreDeploy: func(_ context.Context, _ *spec.UDSBundle, opts *bundle.DeployOptions) error {
				opts.PackageDeployHooks.PreDeploy = func(context.Context, *spec.Package, *bundle.ZarfPackageLayout, *bundle.DeployPackageOptions) error {
					invoked = true
					return hookErr
				}
				return nil
			},
		},
	})

	require.ErrorIs(t, err, hookErr)
	assert.True(t, invoked)
}

func TestDeployValidatesOptionsBeforeHooks(t *testing.T) {
	var invoked bool
	_, err := bundle.Deploy(t.Context(), deploySource(singlePkgBundle()), bundle.DeployOptions{
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy: func(context.Context, *spec.Package, *bundle.ZarfPackageLayout, *bundle.DeployPackageOptions) error {
				invoked = true
				return nil
			},
		},
		Streams: iostreams.IOStreams{},
	})

	require.ErrorContains(t, err, "config is required")
	assert.False(t, invoked)
}
