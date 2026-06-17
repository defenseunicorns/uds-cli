// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"

	bundle "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func mkStaticLoader(pkg *layout.PackageLayout, err error) bundle.PackageLayoutLoader {
	return &staticLoaderImpl{pkg: pkg, err: err}
}

type staticLoaderImpl struct {
	pkg *layout.PackageLayout
	err error
}

func (l *staticLoaderImpl) LoadPackageLayout(_ context.Context, _ *bundle.Package, _ string, _ bundle.LoadOptions) (*layout.PackageLayout, error) {
	return l.pkg, l.err
}

func imageLayoutLib() *layout.PackageLayout {
	return &layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{
			Components: []v1alpha1.ZarfComponent{
				{
					Name:   "main",
					Images: []string{"ghcr.io/example/image:v1"},
					ImageArchives: []v1alpha1.ImageArchive{
						{Path: "archive.tar", Images: []string{"ghcr.io/example/image:v1"}},
					},
				},
			},
		},
	}
}

func singlePkgBundle() *bundle.UDSBundle {
	return &bundle.UDSBundle{
		UDS:      bundle.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: bundle.Metadata{Name: "test-bundle"},
		Packages: []bundle.Package{
			{Name: "alpha", Source: "oci://example.com/alpha:v1"},
		},
	}
}

func twoPkgLinearBundle() *bundle.UDSBundle {
	return &bundle.UDSBundle{
		UDS:      bundle.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: bundle.Metadata{Name: "test-bundle"},
		Packages: []bundle.Package{
			{Name: "alpha", Source: "oci://example.com/alpha:v1"},
			{Name: "beta", Source: "oci://example.com/beta:v1", DependsOn: []bundle.PackageRef{{Name: "alpha"}}},
		},
	}
}

// newTestConfig returns a UDSBundleConfig with defaults suitable for these tests.
func newTestConfig() *bundle.UDSBundleConfig {
	return &bundle.UDSBundleConfig{
		Global:  &bundle.GlobalOptions{},
		Options: &bundle.ConfigOptions{Architecture: runtime.GOARCH, TmpDir: os.TempDir(), Concurrency: 10},
	}
}

// noopClusterDeploy is a cluster-deploy stand-in that does nothing, replacing the real
// packager.Deploy so these tests exercise the hook wiring without a cluster.
func noopClusterDeploy(context.Context, *layout.PackageLayout, packager.DeployOptions) error {
	return nil
}

// deployWithClusterStub returns a DeployOptions.PackageDeployFn that runs the deployer's
// real DeployPackage (loader + hooks) with cluster injected as the cluster-side deploy, so
// bundle-level tests exercise the full per-package pipeline without touching a cluster.
func deployWithClusterStub(d *bundle.ZarfDeployer, cluster func(context.Context, *layout.PackageLayout, packager.DeployOptions) error) func(context.Context, *bundle.Package, bundle.DeployPackageOptions) error {
	return func(ctx context.Context, pkg *bundle.Package, opts bundle.DeployPackageOptions) error {
		opts.ClusterDeployFn = cluster
		return d.DeployPackage(ctx, pkg, opts)
	}
}

func serialConfig() *bundle.UDSBundleConfig {
	cfg := newTestConfig()
	cfg.Options.Concurrency = 1
	return cfg
}

func TestPackageHooks_DefaultNoOp(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))
	var deployCalled int

	err := d.DeployPackage(t.Context(), &bundle.Package{Name: "alpha", Source: "oci://x"}, bundle.DeployPackageOptions{
		Config:    newTestConfig(),
		BundleDir: t.TempDir(),
		ClusterDeployFn: func(context.Context, *layout.PackageLayout, packager.DeployOptions) error {
			deployCalled++
			return nil
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, deployCalled, "ClusterDeployFn must be called exactly once")
}

func TestPackageHooks_PreDeployImageSkip(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))

	var capturedLayout *layout.PackageLayout
	err := d.DeployPackage(t.Context(), &bundle.Package{Name: "alpha", Source: "oci://x"}, bundle.DeployPackageOptions{
		Config:    newTestConfig(),
		BundleDir: t.TempDir(),
		ClusterDeployFn: func(_ context.Context, l *layout.PackageLayout, _ packager.DeployOptions) error {
			capturedLayout = l
			return nil
		},
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy: func(_ context.Context, _ *bundle.Package, pkgLayout *layout.PackageLayout, _ *packager.DeployOptions, _ *bundle.DeployPackageOptions) error {
				for i := range pkgLayout.Pkg.Components {
					pkgLayout.Pkg.Components[i].Images = []string{}
					pkgLayout.Pkg.Components[i].ImageArchives = []v1alpha1.ImageArchive{}
				}
				return nil
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, capturedLayout)
	for _, c := range capturedLayout.Pkg.Components {
		assert.Empty(t, c.Images, "component %q: images must be zeroed", c.Name)
		assert.Empty(t, c.ImageArchives, "component %q: image archives must be zeroed", c.Name)
	}
}

func TestPackageHooks_PostDeployTracking(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))

	var tracked []string
	err := d.DeployPackage(t.Context(), &bundle.Package{Name: "alpha", Source: "oci://x"}, bundle.DeployPackageOptions{
		Config:          newTestConfig(),
		BundleDir:       t.TempDir(),
		ClusterDeployFn: noopClusterDeploy,
		PackageDeployHooks: bundle.PackageDeployHooks{
			PostDeploy: func(_ context.Context, pkg *bundle.Package) error {
				tracked = append(tracked, pkg.Name)
				return nil
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha"}, tracked)
}

func TestPackageHooks_PreDeployErrorAbortsBeforeDeploy(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))

	var deployCalled bool
	hookErr := errors.New("pre-deploy hook failed")
	err := d.DeployPackage(t.Context(), &bundle.Package{Name: "alpha", Source: "oci://x"}, bundle.DeployPackageOptions{
		Config:    newTestConfig(),
		BundleDir: t.TempDir(),
		ClusterDeployFn: func(context.Context, *layout.PackageLayout, packager.DeployOptions) error {
			deployCalled = true
			return nil
		},
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy: func(context.Context, *bundle.Package, *layout.PackageLayout, *packager.DeployOptions, *bundle.DeployPackageOptions) error {
				return hookErr
			},
		},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, hookErr)
	assert.Contains(t, err.Error(), `"alpha"`)
	assert.False(t, deployCalled, "ClusterDeployFn must not be called when PreDeploy errors")
}

func TestPackageHooks_PostDeployErrorSurfaces(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))

	postErr := errors.New("post-deploy error")
	err := d.DeployPackage(t.Context(), &bundle.Package{Name: "alpha", Source: "oci://x"}, bundle.DeployPackageOptions{
		Config:          newTestConfig(),
		BundleDir:       t.TempDir(),
		ClusterDeployFn: noopClusterDeploy,
		PackageDeployHooks: bundle.PackageDeployHooks{
			PostDeploy: func(context.Context, *bundle.Package) error { return postErr },
		},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, postErr)
}

func TestPackageHooks_Ordering(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))

	var order []string
	var mu sync.Mutex
	record := func(step string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, step)
	}

	err := d.DeployPackage(t.Context(), &bundle.Package{Name: "alpha", Source: "oci://x"}, bundle.DeployPackageOptions{
		Config:    newTestConfig(),
		BundleDir: t.TempDir(),
		ClusterDeployFn: func(context.Context, *layout.PackageLayout, packager.DeployOptions) error {
			record("deploy")
			return nil
		},
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy: func(context.Context, *bundle.Package, *layout.PackageLayout, *packager.DeployOptions, *bundle.DeployPackageOptions) error {
				record("PreDeploy")
				return nil
			},
			PostDeploy: func(context.Context, *bundle.Package) error {
				record("PostDeploy")
				return nil
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"PreDeploy", "deploy", "PostDeploy"}, order)
}

func TestPackageHooks_DeployFailureSkipsPostDeploy(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))

	deployErr := errors.New("cluster unavailable")
	var postCalled bool
	err := d.DeployPackage(t.Context(), &bundle.Package{Name: "alpha", Source: "oci://x"}, bundle.DeployPackageOptions{
		Config:    newTestConfig(),
		BundleDir: t.TempDir(),
		ClusterDeployFn: func(context.Context, *layout.PackageLayout, packager.DeployOptions) error {
			return deployErr
		},
		PackageDeployHooks: bundle.PackageDeployHooks{
			PostDeploy: func(context.Context, *bundle.Package) error {
				postCalled = true
				return nil
			},
		},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, deployErr)
	assert.Contains(t, err.Error(), `failed to deploy package "alpha"`)
	assert.False(t, postCalled, "PostDeploy must not be called when deploy errors")
}

func TestBundleHooks_FireOnceInOrder(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))

	var order []string
	var mu sync.Mutex
	record := func(step string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, step)
	}

	b := twoPkgLinearBundle()
	_, err := d.DeployBundle(t.Context(), b, bundle.DeployOptions{
		Config: serialConfig(),
		PackageDeployFn: deployWithClusterStub(d, func(context.Context, *layout.PackageLayout, packager.DeployOptions) error {
			record("deploy")
			return nil
		}),
		BundleDeployHooks: bundle.BundleDeployHooks{
			PreDeploy: func(_ context.Context, _ *bundle.UDSBundle, _ *bundle.DeployOptions) error {
				record("Bundle.PreDeploy")
				return nil
			},
			PostDeploy: func(_ context.Context, _ *bundle.UDSBundle) error { record("Bundle.PostDeploy"); return nil },
		},
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy: func(_ context.Context, pkg *bundle.Package, _ *layout.PackageLayout, _ *packager.DeployOptions, _ *bundle.DeployPackageOptions) error {
				record(fmt.Sprintf("Pkg.PreDeploy(%s)", pkg.Name))
				return nil
			},
			PostDeploy: func(_ context.Context, pkg *bundle.Package) error {
				record(fmt.Sprintf("Pkg.PostDeploy(%s)", pkg.Name))
				return nil
			},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"Bundle.PreDeploy",
		"Pkg.PreDeploy(alpha)", "deploy", "Pkg.PostDeploy(alpha)",
		"Pkg.PreDeploy(beta)", "deploy", "Pkg.PostDeploy(beta)",
		"Bundle.PostDeploy",
	}, order)
}

func TestBundleHooks_PreDeployMutatesOpts(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))

	var pkgPostCalled []string
	var mu sync.Mutex

	b := singlePkgBundle()
	_, err := d.DeployBundle(t.Context(), b, bundle.DeployOptions{
		Config:          newTestConfig(),
		PackageDeployFn: deployWithClusterStub(d, noopClusterDeploy),
		BundleDeployHooks: bundle.BundleDeployHooks{
			PreDeploy: func(_ context.Context, _ *bundle.UDSBundle, opts *bundle.DeployOptions) error {
				opts.PackageDeployHooks = bundle.PackageDeployHooks{
					PostDeploy: func(_ context.Context, pkg *bundle.Package) error {
						mu.Lock()
						defer mu.Unlock()
						pkgPostCalled = append(pkgPostCalled, pkg.Name)
						return nil
					},
				}
				return nil
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha"}, pkgPostCalled, "package hooks installed by Bundle.PreDeploy must run")
}

func TestBundleHooks_PreDeployErrorAbortsBeforeAnyPackage(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))

	var deployCalled bool
	preErr := errors.New("pre-deploy error")
	result, err := d.DeployBundle(t.Context(), singlePkgBundle(), bundle.DeployOptions{
		Config: newTestConfig(),
		PackageDeployFn: deployWithClusterStub(d, func(context.Context, *layout.PackageLayout, packager.DeployOptions) error {
			deployCalled = true
			return nil
		}),
		BundleDeployHooks: bundle.BundleDeployHooks{
			PreDeploy: func(context.Context, *bundle.UDSBundle, *bundle.DeployOptions) error { return preErr },
		},
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, preErr)
	assert.Contains(t, err.Error(), "pre-deploy bundle hook failed")
	assert.False(t, deployCalled, "no package deploy must run when Bundle.PreDeploy errors")
}

func TestBundleHooks_PostDeployErrorAfterSuccessReturnsResult(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))

	postErr := errors.New("post-deploy error")
	result, err := d.DeployBundle(t.Context(), singlePkgBundle(), bundle.DeployOptions{
		Config:          newTestConfig(),
		PackageDeployFn: deployWithClusterStub(d, noopClusterDeploy),
		BundleDeployHooks: bundle.BundleDeployHooks{
			PostDeploy: func(context.Context, *bundle.UDSBundle) error { return postErr },
		},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, postErr)
	assert.Contains(t, err.Error(), "post-deploy bundle hook failed")
	// Packages are already deployed when PostDeploy runs, so the result is returned
	// alongside the error (not nil) — see ADR 0013 and BundleDeployHooks.PostDeploy.
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Packages, "deployed packages must still be reported")
}
