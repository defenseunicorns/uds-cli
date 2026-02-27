// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockDeployer is a test double for the Deployer interface.
type MockDeployer struct {
	// DeployedPackages tracks the packages that were deployed in order
	DeployedPackages []*Package

	// FailOnPackage causes DeployPackage to fail for a specific package name
	FailOnPackage string

	// FailError is the error to return when FailOnPackage matches
	FailError error
}

func (m *MockDeployer) DeployPackage(_ context.Context, pkg *Package, _ DeployPackageOptions) error {
	if m.FailOnPackage == pkg.Name && m.FailError != nil {
		return m.FailError
	}
	m.DeployedPackages = append(m.DeployedPackages, pkg)
	return nil
}

func TestDeployOrder_InitBundle(t *testing.T) {
	// Test the specific init bundle scenario with two packages and one dependency
	bundle := &UDSBundle{
		Packages: []Package{
			{Name: "uds_k3d_dev", Source: "oci://ghcr.io/example/k3d:v1"},
			{Name: "init", Source: "oci://ghcr.io/example/init:v1", DependsOn: []PackageRef{{Name: "uds_k3d_dev"}}},
		},
	}

	dag, err := BuildDependencyGraph(bundle)
	require.NoError(t, err)

	// Test topological sort (flat order)
	sorted, err := dag.TopologicalSort()
	require.NoError(t, err)

	var sortedNames []string
	for _, pkg := range sorted {
		sortedNames = append(sortedNames, pkg.Name)
	}
	assert.Equal(t, []string{"uds_k3d_dev", "init"}, sortedNames, "uds_k3d_dev should deploy before init")

	// Test topological levels
	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)
	assert.Len(t, levels, 2, "should have 2 deployment levels")

	// Level 0 should contain uds_k3d_dev
	assert.Len(t, levels[0], 1)
	assert.Equal(t, "uds_k3d_dev", levels[0][0].Name)

	// Level 1 should contain init
	assert.Len(t, levels[1], 1)
	assert.Equal(t, "init", levels[1][0].Name)
}

func TestDeployOrder_SinglePackage(t *testing.T) {
	// Test bundle with single package (no dependencies)
	bundle := &UDSBundle{
		Packages: []Package{
			{Name: "standalone", Source: "oci://ghcr.io/example/pkg:v1"},
		},
	}

	dag, err := BuildDependencyGraph(bundle)
	require.NoError(t, err)

	sorted, err := dag.TopologicalSort()
	require.NoError(t, err)

	assert.Len(t, sorted, 1)
	assert.Equal(t, "standalone", sorted[0].Name)

	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)
	assert.Len(t, levels, 1)
}

func TestDeployOrder_LinearChain(t *testing.T) {
	// Test linear chain: A -> B -> C
	bundle := &UDSBundle{
		Packages: []Package{
			{Name: "pkg_c", Source: "oci://example/c:v1", DependsOn: []PackageRef{{Name: "pkg_b"}}},
			{Name: "pkg_b", Source: "oci://example/b:v1", DependsOn: []PackageRef{{Name: "pkg_a"}}},
			{Name: "pkg_a", Source: "oci://example/a:v1"},
		},
	}

	dag, err := BuildDependencyGraph(bundle)
	require.NoError(t, err)

	sorted, err := dag.TopologicalSort()
	require.NoError(t, err)

	var sortedNames []string
	for _, pkg := range sorted {
		sortedNames = append(sortedNames, pkg.Name)
	}
	assert.Equal(t, []string{"pkg_a", "pkg_b", "pkg_c"}, sortedNames)

	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)
	assert.Len(t, levels, 3, "linear chain should have 3 levels")
}

func TestDeployOrder_DiamondPattern(t *testing.T) {
	// Test diamond pattern: base -> (left, right) -> top
	bundle := &UDSBundle{
		Packages: []Package{
			{Name: "base", Source: "oci://example/base:v1"},
			{Name: "left", Source: "oci://example/left:v1", DependsOn: []PackageRef{{Name: "base"}}},
			{Name: "right", Source: "oci://example/right:v1", DependsOn: []PackageRef{{Name: "base"}}},
			{Name: "top", Source: "oci://example/top:v1", DependsOn: []PackageRef{{Name: "left"}, {Name: "right"}}},
		},
	}

	dag, err := BuildDependencyGraph(bundle)
	require.NoError(t, err)

	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)

	assert.Len(t, levels, 3, "diamond should have 3 levels")

	// Level 0: base
	assert.Len(t, levels[0], 1)
	assert.Equal(t, "base", levels[0][0].Name)

	// Level 1: left and right (order may vary, both should be in level 1)
	assert.Len(t, levels[1], 2)
	level1Names := []string{levels[1][0].Name, levels[1][1].Name}
	assert.Contains(t, level1Names, "left")
	assert.Contains(t, level1Names, "right")

	// Level 2: top
	assert.Len(t, levels[2], 1)
	assert.Equal(t, "top", levels[2][0].Name)
}

func TestMockDeployer_TracksDeploys(t *testing.T) {
	mock := &MockDeployer{}

	pkgs := []*Package{
		{Name: "pkg1"},
		{Name: "pkg2"},
	}

	for _, pkg := range pkgs {
		err := mock.DeployPackage(context.Background(), pkg, DeployPackageOptions{})
		require.NoError(t, err)
	}

	assert.Len(t, mock.DeployedPackages, 2)
	assert.Equal(t, "pkg1", mock.DeployedPackages[0].Name)
	assert.Equal(t, "pkg2", mock.DeployedPackages[1].Name)
}

func TestMockDeployer_CanFail(t *testing.T) {
	mock := &MockDeployer{
		FailOnPackage: "fail-me",
		FailError:     assert.AnError,
	}

	err := mock.DeployPackage(context.Background(), &Package{Name: "success"}, DeployPackageOptions{})
	require.NoError(t, err)

	err = mock.DeployPackage(context.Background(), &Package{Name: "fail-me"}, DeployPackageOptions{})
	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}
