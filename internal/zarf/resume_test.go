// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"errors"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/state"
)

type packageSpecLoaderFunc func(context.Context, *spec.Package) (*PackageSpec, error)

func (f packageSpecLoaderFunc) LoadPackageSpec(ctx context.Context, pkg *spec.Package) (*PackageSpec, error) {
	return f(ctx, pkg)
}

func TestResumeMatches(t *testing.T) {
	intended := &PackageSpec{Name: "zarf-name", Digest: "sha256:one", Components: []string{"required", "optional"}}
	matching := []state.DeployedPackage{{Digest: "sha256:one", DeployedComponents: []state.DeployedComponent{{Name: "optional", Status: state.ComponentStatusSucceeded}, {Name: "required", Status: state.ComponentStatusSucceeded}}}}
	redeploy := []struct {
		name     string
		intended *PackageSpec
		deployed []state.DeployedPackage
	}{
		{name: "missing state"},
		{name: "missing intended digest", intended: &PackageSpec{Name: "zarf-name", Components: intended.Components}, deployed: matching},
		{name: "missing deployed digest", intended: intended, deployed: []state.DeployedPackage{{DeployedComponents: matching[0].DeployedComponents}}},
		{name: "different digest", intended: intended, deployed: []state.DeployedPackage{{Digest: "sha256:two", DeployedComponents: matching[0].DeployedComponents}}},
		{name: "failed component", intended: intended, deployed: []state.DeployedPackage{{Digest: "sha256:one", DeployedComponents: []state.DeployedComponent{{Name: "required", Status: state.ComponentStatusSucceeded}, {Name: "optional", Status: state.ComponentStatusDeploying}}}}},
		{name: "missing component", intended: intended, deployed: []state.DeployedPackage{{Digest: "sha256:one", DeployedComponents: matching[0].DeployedComponents[:1]}}},
		{name: "extra component", intended: intended, deployed: []state.DeployedPackage{{Digest: "sha256:one", DeployedComponents: append(matching[0].DeployedComponents, state.DeployedComponent{Name: "extra", Status: state.ComponentStatusSucceeded})}}},
		{name: "duplicate deployed component", intended: intended, deployed: []state.DeployedPackage{{Digest: "sha256:one", DeployedComponents: []state.DeployedComponent{{Name: "required", Status: state.ComponentStatusSucceeded}, {Name: "required", Status: state.ComponentStatusSucceeded}}}}},
		{name: "duplicate intended component", intended: &PackageSpec{Name: "zarf-name", Digest: "sha256:one", Components: []string{"required", "required"}}, deployed: matching},
		{name: "duplicate deployed package", intended: intended, deployed: append(matching, matching[0])},
	}

	assert.True(t, resumeMatches(intended, matching))
	for _, tt := range redeploy {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, resumeMatches(tt.intended, tt.deployed))
		})
	}
}

func TestFilterResumeLevels_UsesBatchStateAndZarfIdentity(t *testing.T) {
	levels := [][]*spec.Package{{{Name: "hcl-label", Namespace: "team-a"}}}
	stateCalls := 0
	loaderCalls := 0
	filtered, err := filterResumeLevels(t.Context(), levels, DeployOptions{
		SpecLoader: packageSpecLoaderFunc(func(_ context.Context, pkg *spec.Package) (*PackageSpec, error) {
			loaderCalls++
			assert.Equal(t, "hcl-label", pkg.Name)
			return &PackageSpec{Name: "zarf-name", Digest: "sha256:one", Components: []string{"main"}}, nil
		}),
		DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) {
			stateCalls++
			return []state.DeployedPackage{{Name: "zarf-name", NamespaceOverride: "team-a", Digest: "sha256:one", DeployedComponents: []state.DeployedComponent{{Name: "main", Status: state.ComponentStatusSucceeded}}}}, nil
		},
	}, iostreams.IOStreams{})
	require.NoError(t, err)
	assert.Equal(t, 1, stateCalls)
	assert.Equal(t, 1, loaderCalls)
	assert.Empty(t, filtered)
}

func TestFilterResumeLevels_RedeploysNamespaceMismatch(t *testing.T) {
	levels := [][]*spec.Package{{{Name: "pkg", Namespace: "team-a"}}}
	filtered, err := filterResumeLevels(t.Context(), levels, DeployOptions{
		SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) {
			return &PackageSpec{Name: "zarf-name", Digest: "sha256:one", Components: []string{"main"}}, nil
		}),
		DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) {
			return []state.DeployedPackage{{Name: "zarf-name", NamespaceOverride: "team-b", Digest: "sha256:one", DeployedComponents: []state.DeployedComponent{{Name: "main", Status: state.ComponentStatusSucceeded}}}}, nil
		},
	}, iostreams.IOStreams{})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "pkg", filtered[0][0].Name)
}

func TestDeployBundleResumeOrderAndFailures(t *testing.T) {
	bundle := &spec.UDSBundle{UDS: spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"}, Metadata: spec.Metadata{Name: "bundle"}, Packages: []spec.Package{{Name: "core", Source: "oci://example.com/core:v1"}, {Name: "app", Source: "oci://example.com/app:v1", DependsOn: []spec.PackageRef{{Name: "core"}}}}}
	t.Run("disabled performs no state or spec reads", func(t *testing.T) {
		stateCalls, specCalls, deployCalls := 0, 0, 0
		result, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(t.Context(), bundle, DeployOptions{
			Config: newDeployTestConfig(1),
			SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) {
				specCalls++
				return nil, errors.New("unexpected spec read")
			}),
			DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) {
				stateCalls++
				return nil, errors.New("unexpected state read")
			},
			PackageDeployFn: func(context.Context, *spec.Package, DeployPackageOptions) error { deployCalls++; return nil },
		})
		require.NoError(t, err)
		assert.Equal(t, 0, stateCalls)
		assert.Equal(t, 0, specCalls)
		assert.Equal(t, 2, deployCalls)
		assert.Len(t, result.Packages, 2)
	})

	t.Run("selection happens before spec loading", func(t *testing.T) {
		var loaded []string
		_, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(t.Context(), bundle, DeployOptions{
			Config:   newDeployTestConfig(1),
			Resume:   true,
			Packages: []string{"app"},
			SpecLoader: packageSpecLoaderFunc(func(_ context.Context, pkg *spec.Package) (*PackageSpec, error) {
				loaded = append(loaded, pkg.Name)
				return &PackageSpec{Name: pkg.Name, Digest: "sha256:one"}, nil
			}),
			DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) { return nil, nil },
			PackageDeployFn:    func(context.Context, *spec.Package, DeployPackageOptions) error { return nil },
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"app"}, loaded)
	})

	t.Run("bundle pre-deploy config mutations feed resume loader", func(t *testing.T) {
		pkgDir := t.TempDir()
		writeMinimalZarfPackage(t, pkgDir, "core")
		deployer := NewZarfDeployer(iostreams.IOStreams{}, nil)
		stateCalls, deployCalls := 0, 0
		localBundle := &spec.UDSBundle{
			UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
			Metadata: spec.Metadata{Name: "bundle"},
			Packages: []spec.Package{{Name: "core", Source: pkgDir}},
		}
		result, err := deployer.DeployBundle(t.Context(), localBundle, DeployOptions{
			Config:     newDeployTestConfig(1),
			Resume:     true,
			BundlePath: "/tmp/bundle/bundle.uds.hcl",
			BundleDeployHooks: BundleDeployHooks{
				PreDeploy: func(_ context.Context, _ *spec.UDSBundle, opts *DeployOptions) error {
					opts.Config.Options.Architecture = "arm64"
					opts.Config.Options.PlainHTTP = true
					return nil
				},
			},
			DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) {
				stateCalls++
				return nil, nil
			},
			PackageDeployFn: func(context.Context, *spec.Package, DeployPackageOptions) error {
				deployCalls++
				loader, ok := deployer.Loader.(*SourcePackageLayoutLoader)
				require.True(t, ok)
				assert.Equal(t, "arm64", loader.configOpts.Architecture)
				assert.True(t, loader.configOpts.PlainHTTP)
				return nil
			},
		})
		require.NoError(t, err)
		assert.Equal(t, 1, stateCalls)
		assert.Equal(t, 1, deployCalls)
		assert.Len(t, result.Packages, 1)
	})

	t.Run("resume rejects package pre-deploy hooks after bundle pre-deploy", func(t *testing.T) {
		hookCalls, stateCalls, deployCalls := 0, 0, 0
		_, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(t.Context(), bundle, DeployOptions{
			Config:     newDeployTestConfig(1),
			Resume:     true,
			BundlePath: "/tmp/bundle/bundle.uds.hcl",
			BundleDeployHooks: BundleDeployHooks{
				PreDeploy: func(context.Context, *spec.UDSBundle, *DeployOptions) error {
					hookCalls++
					return nil
				},
			},
			PackageDeployHooks: PackageDeployHooks{
				PreDeploy: func(context.Context, *spec.Package, *layout.PackageLayout, *packager.DeployOptions, *DeployPackageOptions) error {
					return nil
				},
			},
			DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) {
				stateCalls++
				return nil, nil
			},
			PackageDeployFn: func(context.Context, *spec.Package, DeployPackageOptions) error {
				deployCalls++
				return nil
			},
		})
		require.ErrorIs(t, err, ErrResumeWithPackageHook)
		assert.Equal(t, 1, hookCalls)
		assert.Zero(t, stateCalls)
		assert.Zero(t, deployCalls)
	})

	for _, tt := range []struct {
		name string
		opts DeployOptions
	}{
		{name: "state read", opts: DeployOptions{DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) { return nil, errors.New("state") }}},
		{name: "spec read", opts: DeployOptions{DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) { return nil, nil }, SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) { return nil, errors.New("spec") })}},
		{name: "missing digest", opts: DeployOptions{DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) { return nil, nil }, SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) { return &PackageSpec{Name: "pkg"}, nil })}},
	} {
		t.Run(tt.name+" aborts before hooks and deploy", func(t *testing.T) {
			hookCalls, deployCalls := 0, 0
			tt.opts.Config = newDeployTestConfig(1)
			tt.opts.BundlePath = "/tmp/bundle/bundle.uds.hcl"
			tt.opts.Resume = true
			tt.opts.BundleDeployHooks.PreDeploy = func(context.Context, *spec.UDSBundle, *DeployOptions) error { hookCalls++; return nil }
			tt.opts.PackageDeployFn = func(context.Context, *spec.Package, DeployPackageOptions) error { deployCalls++; return nil }
			_, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(t.Context(), bundle, tt.opts)
			require.Error(t, err)
			assert.Equal(t, 1, hookCalls)
			assert.Zero(t, deployCalls)
		})
	}
}

func TestDeployBundleResume_AllSkippedRunsHooks(t *testing.T) {
	bundle := &spec.UDSBundle{UDS: spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"}, Metadata: spec.Metadata{Name: "bundle"}, Packages: []spec.Package{{Name: "label", Namespace: "team", Source: "oci://example.com/pkg:v1"}}}
	pre, post, deployed := 0, 0, 0
	result, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(t.Context(), bundle, DeployOptions{
		Config: newDeployTestConfig(1), Resume: true,
		SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) {
			return &PackageSpec{Name: "zarf-name", Digest: "sha256:one", Components: []string{"main"}}, nil
		}),
		DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) {
			return []state.DeployedPackage{{Name: "zarf-name", NamespaceOverride: "team", Digest: "sha256:one", DeployedComponents: []state.DeployedComponent{{Name: "main", Status: state.ComponentStatusSucceeded}}}}, nil
		},
		BundleDeployHooks: BundleDeployHooks{PreDeploy: func(context.Context, *spec.UDSBundle, *DeployOptions) error { pre++; return nil }, PostDeploy: func(context.Context, *spec.UDSBundle) error { post++; return nil }},
		PackageDeployFn:   func(context.Context, *spec.Package, DeployPackageOptions) error { deployed++; return nil },
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.Packages)
	assert.Empty(t, result.Packages)
	assert.Equal(t, 1, pre)
	assert.Equal(t, 1, post)
	assert.Zero(t, deployed)
}
