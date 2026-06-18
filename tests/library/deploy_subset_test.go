// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundle "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// threePkgChainBundle returns a bundle with a three-package linear dependency chain:
//
//	base (no deps)
//	middle depends on base
//	leaf   depends on middle
func threePkgChainBundle() *bundle.UDSBundle {
	return &bundle.UDSBundle{
		UDS:      bundle.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: bundle.Metadata{Name: "subset-test-bundle"},
		Packages: []bundle.Package{
			{Name: "base", Source: "oci://example.com/base:v1"},
			{Name: "middle", Source: "oci://example.com/middle:v1", DependsOn: []bundle.PackageRef{{Name: "base"}}},
			{Name: "leaf", Source: "oci://example.com/leaf:v1", DependsOn: []bundle.PackageRef{{Name: "middle"}}},
		},
	}
}

// recorderFn returns a PackageDeployFn that records deployed package names in order.
func recorderFn(mu *sync.Mutex, deployed *[]string) func(context.Context, *bundle.Package, bundle.DeployPackageOptions) error {
	return func(_ context.Context, pkg *bundle.Package, _ bundle.DeployPackageOptions) error {
		mu.Lock()
		defer mu.Unlock()
		*deployed = append(*deployed, pkg.Name)
		return nil
	}
}

// TestDeploySubset_FullBundle verifies that an empty Packages set deploys all packages.
func TestDeploySubset_FullBundle(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))
	var mu sync.Mutex
	var deployed []string

	b := threePkgChainBundle()
	result, err := d.DeployBundle(t.Context(), b, bundle.DeployOptions{
		Config:          serialConfig(),
		PackageDeployFn: recorderFn(&mu, &deployed),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"base", "middle", "leaf"}, deployed)
	assert.Equal(t, 3, result.Packages, "result should report all 3 packages deployed")
}

// TestDeploySubset_SubsetInOrder verifies that a root+middle subset deploys only those two.
func TestDeploySubset_SubsetInOrder(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))
	var mu sync.Mutex
	var deployed []string

	b := threePkgChainBundle()
	result, err := d.DeployBundle(t.Context(), b, bundle.DeployOptions{
		Config:          serialConfig(),
		Packages:        []string{"base", "middle"},
		PackageDeployFn: recorderFn(&mu, &deployed),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	// base must come before middle (topological order)
	assert.Equal(t, []string{"base", "middle"}, deployed)
	assert.Equal(t, 2, result.Packages, "result.Packages should equal the deployed subset count (2)")
}

// TestDeploySubset_LeafWithoutDepBlocked verifies selecting a dependent whose
// dependency is unselected returns an error and deploys nothing.
func TestDeploySubset_LeafWithoutDepBlocked(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))
	var mu sync.Mutex
	var deployed []string

	b := threePkgChainBundle()
	result, err := d.DeployBundle(t.Context(), b, bundle.DeployOptions{
		Config:          serialConfig(),
		Packages:        []string{"leaf"},
		PackageDeployFn: recorderFn(&mu, &deployed),
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "requires")
	assert.Empty(t, deployed, "no packages should be deployed when safety check fails")
}

// TestDeploySubset_ForceBypassesSafetyCheck verifies that Force=true allows
// deploying a package whose dependencies are not selected.
func TestDeploySubset_ForceBypassesSafetyCheck(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))
	var mu sync.Mutex
	var deployed []string

	b := threePkgChainBundle()
	result, err := d.DeployBundle(t.Context(), b, bundle.DeployOptions{
		Config:          serialConfig(),
		Packages:        []string{"leaf"},
		Force:           true,
		PackageDeployFn: recorderFn(&mu, &deployed),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"leaf"}, deployed)
}

// TestDeploySubset_UnknownPackage verifies that requesting a package name not
// in the bundle returns a clear error.
func TestDeploySubset_UnknownPackage(t *testing.T) {
	d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))
	var mu sync.Mutex
	var deployed []string

	b := threePkgChainBundle()
	result, err := d.DeployBundle(t.Context(), b, bundle.DeployOptions{
		Config:          serialConfig(),
		Packages:        []string{"nonexistent"},
		PackageDeployFn: recorderFn(&mu, &deployed),
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unknown packages")
	assert.Empty(t, deployed)
}

// TestDeploySubset_PreDeployHookGatedByValidation verifies that the bundle
// PreDeploy hook fires only after the package selection is validated: it must
// NOT run when the selection is invalid (unknown name or unselected dependency
// without --force), so side-effecting hooks are not triggered for a request that
// is about to be rejected; it MUST run for a valid selection.
func TestDeploySubset_PreDeployHookGatedByValidation(t *testing.T) {
	tests := []struct {
		name            string
		packages        []string
		force           bool
		wantErr         bool
		wantHookInvoked bool
	}{
		{name: "unknown package skips hook", packages: []string{"nonexistent"}, wantErr: true, wantHookInvoked: false},
		{name: "unselected dependency skips hook", packages: []string{"leaf"}, wantErr: true, wantHookInvoked: false},
		{name: "valid subset fires hook", packages: []string{"base", "middle"}, wantErr: false, wantHookInvoked: true},
		{name: "forced out-of-order fires hook", packages: []string{"leaf"}, force: true, wantErr: false, wantHookInvoked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := bundle.NewZarfDeployer(iostreams.IOStreams{}, mkStaticLoader(imageLayoutLib(), nil))
			var mu sync.Mutex
			var deployed []string
			hookInvoked := false

			b := threePkgChainBundle()
			_, err := d.DeployBundle(t.Context(), b, bundle.DeployOptions{
				Config:          serialConfig(),
				Packages:        tt.packages,
				Force:           tt.force,
				PackageDeployFn: recorderFn(&mu, &deployed),
				BundleDeployHooks: bundle.BundleDeployHooks{
					PreDeploy: func(context.Context, *bundle.UDSBundle, *bundle.DeployOptions) error {
						hookInvoked = true
						return nil
					},
				},
			})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantHookInvoked, hookInvoked, "PreDeploy hook invocation should be gated by package validation")
		})
	}
}
