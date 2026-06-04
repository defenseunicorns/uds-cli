// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These deploy-order tests exercise the bundle → DAG → topological-levels
// path that ZarfDeployer.DeployBundle relies on. They overlap with cases in
// dag_test.go on purpose: kept here so any future change to the DAG keeps
// the deploy-side guarantees (correct ordering, level grouping) regressed
// against. Pure orchestration scheduling lives in deploy_orchestrator_test.go.

func TestApplyValuesFilesOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pkgs     []Package
		override map[string][]string
		want     [][]string
	}{
		{
			name: "package in map gets override paths",
			pkgs: []Package{
				{Name: "pkg-a", ValuesFiles: []string{"original-a.yaml"}},
			},
			override: map[string][]string{
				"pkg-a": {"values/0.yaml"},
			},
			want: [][]string{{"values/0.yaml"}},
		},
		{
			name: "package missing from map gets nil, not HCL fallback",
			pkgs: []Package{
				{Name: "pkg-a", ValuesFiles: []string{"original-a.yaml"}},
				{Name: "pkg-b", ValuesFiles: []string{"original-b.yaml"}},
			},
			override: map[string][]string{
				"pkg-a": {"values/0.yaml"},
				// pkg-b intentionally absent
			},
			want: [][]string{{"values/0.yaml"}, nil},
		},
		{
			name: "all packages missing from map get nil",
			pkgs: []Package{
				{Name: "pkg-a", ValuesFiles: []string{"original-a.yaml"}},
			},
			override: map[string][]string{},
			want:     [][]string{nil},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			applyValuesFilesOverride(tc.pkgs, tc.override)
			for i, pkg := range tc.pkgs {
				assert.Equal(t, tc.want[i], pkg.ValuesFiles)
			}
		})
	}
}

func TestDeployOrder_InitBundle(t *testing.T) {
	bundle := &UDSBundle{
		Packages: []Package{
			{Name: "uds_k3d_dev", Source: "oci://ghcr.io/example/k3d:v1"},
			{Name: "init", Source: "oci://ghcr.io/example/init:v1", DependsOn: []PackageRef{{Name: "uds_k3d_dev"}}},
		},
	}

	dag, err := BuildDependencyGraph(context.Background(), iostreams.IOStreams{}, bundle)
	require.NoError(t, err)

	sorted, err := dag.TopologicalSort()
	require.NoError(t, err)

	var sortedNames []string
	for _, pkg := range sorted {
		sortedNames = append(sortedNames, pkg.Name)
	}
	assert.Equal(t, []string{"uds_k3d_dev", "init"}, sortedNames, "uds_k3d_dev should deploy before init")

	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)
	assert.Len(t, levels, 2, "should have 2 deployment levels")

	assert.Len(t, levels[0], 1)
	assert.Equal(t, "uds_k3d_dev", levels[0][0].Name)

	assert.Len(t, levels[1], 1)
	assert.Equal(t, "init", levels[1][0].Name)
}

func TestDeployOrder_SinglePackage(t *testing.T) {
	bundle := &UDSBundle{
		Packages: []Package{
			{Name: "standalone", Source: "oci://ghcr.io/example/pkg:v1"},
		},
	}

	dag, err := BuildDependencyGraph(context.Background(), iostreams.IOStreams{}, bundle)
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
	bundle := &UDSBundle{
		Packages: []Package{
			{Name: "pkg_c", Source: "oci://example/c:v1", DependsOn: []PackageRef{{Name: "pkg_b"}}},
			{Name: "pkg_b", Source: "oci://example/b:v1", DependsOn: []PackageRef{{Name: "pkg_a"}}},
			{Name: "pkg_a", Source: "oci://example/a:v1"},
		},
	}

	dag, err := BuildDependencyGraph(context.Background(), iostreams.IOStreams{}, bundle)
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
	bundle := &UDSBundle{
		Packages: []Package{
			{Name: "base", Source: "oci://example/base:v1"},
			{Name: "left", Source: "oci://example/left:v1", DependsOn: []PackageRef{{Name: "base"}}},
			{Name: "right", Source: "oci://example/right:v1", DependsOn: []PackageRef{{Name: "base"}}},
			{Name: "top", Source: "oci://example/top:v1", DependsOn: []PackageRef{{Name: "left"}, {Name: "right"}}},
		},
	}

	dag, err := BuildDependencyGraph(context.Background(), iostreams.IOStreams{}, bundle)
	require.NoError(t, err)

	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)

	assert.Len(t, levels, 3, "diamond should have 3 levels")

	assert.Len(t, levels[0], 1)
	assert.Equal(t, "base", levels[0][0].Name)

	assert.Len(t, levels[1], 2)
	level1Names := []string{levels[1][0].Name, levels[1][1].Name}
	assert.Contains(t, level1Names, "left")
	assert.Contains(t, level1Names, "right")

	assert.Len(t, levels[2], 1)
	assert.Equal(t, "top", levels[2][0].Name)
}
