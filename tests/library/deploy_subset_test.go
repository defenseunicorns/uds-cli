// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	bundle "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func threePkgChainBundle() *spec.UDSBundle {
	return &spec.UDSBundle{
		UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: spec.Metadata{Name: "subset-test-bundle"},
		Packages: []spec.Package{
			{Name: "base", Source: "oci://example.com/base:v1"},
			{Name: "middle", Source: "oci://example.com/middle:v1", DependsOn: []spec.PackageRef{{Name: "base"}}},
			{Name: "leaf", Source: "oci://example.com/leaf:v1", DependsOn: []spec.PackageRef{{Name: "middle"}}},
		},
	}
}

func runSubsetDeploy(t *testing.T, packages []string, force bool) ([]string, error) {
	t.Helper()
	var mu sync.Mutex
	var deployed []string
	hookErr := errors.New("stop after selection")
	_, err := bundle.Deploy(t.Context(), &bundle.DeploySource{
		BundlePath: "/tmp/bundle/bundle.uds.hcl",
		Bundle:     threePkgChainBundle(),
		Loader:     mkStaticLoader(imageLayoutLib(), nil),
	}, bundle.DeployOptions{
		Config:   serialConfig(),
		Packages: packages,
		Force:    force,
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy: func(_ context.Context, pkg *spec.Package, _ *bundle.ZarfPackageLayout, _ *bundle.DeployPackageOptions) error {
				mu.Lock()
				deployed = append(deployed, pkg.Name)
				mu.Unlock()
				return hookErr
			},
		},
	})
	return deployed, err
}

func serialConfig() *bundle.UDSBundleConfig {
	cfg := newTestConfig()
	cfg.Options.Concurrency = 1
	return cfg
}

func TestDeploySubset_FullBundle(t *testing.T) {
	deployed, err := runSubsetDeploy(t, nil, false)
	require.ErrorContains(t, err, "stop after selection")
	assert.Equal(t, []string{"base"}, deployed)
}

func TestDeploySubset_SubsetInOrder(t *testing.T) {
	deployed, err := runSubsetDeploy(t, []string{"base", "middle"}, false)
	require.ErrorContains(t, err, "stop after selection")
	assert.Equal(t, []string{"base"}, deployed)
}

func TestDeploySubset_LeafWithoutDepBlocked(t *testing.T) {
	deployed, err := runSubsetDeploy(t, []string{"leaf"}, false)
	require.ErrorContains(t, err, "requires")
	assert.Empty(t, deployed)
}

func TestDeploySubset_ForceBypassesSafetyCheck(t *testing.T) {
	deployed, err := runSubsetDeploy(t, []string{"leaf"}, true)
	require.ErrorContains(t, err, "stop after selection")
	assert.Equal(t, []string{"leaf"}, deployed)
}

func TestDeploySubset_UnknownPackage(t *testing.T) {
	deployed, err := runSubsetDeploy(t, []string{"nonexistent"}, false)
	require.ErrorContains(t, err, "unknown packages")
	assert.Empty(t, deployed)
}

func TestDeploySubset_PreDeployHookGatedByValidation(t *testing.T) {
	tests := []struct {
		name            string
		packages        []string
		force           bool
		wantErr         string
		wantHookInvoked bool
	}{
		{name: "unknown package skips hook", packages: []string{"nonexistent"}, wantErr: "unknown packages"},
		{name: "unselected dependency skips hook", packages: []string{"leaf"}, wantErr: "requires"},
		{name: "valid subset fires hook", packages: []string{"base", "middle"}, wantErr: "stop after selection", wantHookInvoked: true},
		{name: "forced out-of-order fires hook", packages: []string{"leaf"}, force: true, wantErr: "stop after selection", wantHookInvoked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployed, err := runSubsetDeploy(t, tt.packages, tt.force)
			require.ErrorContains(t, err, tt.wantErr)
			assert.Equal(t, tt.wantHookInvoked, len(deployed) > 0)
		})
	}
}
