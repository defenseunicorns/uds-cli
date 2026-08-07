// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundlehcl

import (
	"testing"

	bundle "github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to build a UDSBundle from a slice of packages.
func bundleWith(pkgs ...Package) *bundle.UDSBundle {
	return &bundle.UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "test"},
		Packages: pkgs,
	}
}

// pkgRef creates a PackageRef from a dependency name.
// This simulates what the HCL parser produces from depends_on = [package.name].
func pkgRef(name string) PackageRef {
	return PackageRef{Name: name}
}

// pkgRefs creates a slice of PackageRefs from dependency names.
func pkgRefs(names ...string) []PackageRef {
	refs := make([]PackageRef, len(names))
	for i, name := range names {
		refs[i] = pkgRef(name)
	}
	return refs
}

// pkg constructs a package with named dependencies for graph tests.
func pkg(name string, deps ...string) Package {
	return Package{Name: name, Source: "oci://example.com/" + name + ":v1", DependsOn: pkgRefs(deps...)}
}

func TestBuildDependencyGraph_SinglePackage(t *testing.T) {
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(pkg("a")))
	require.NoError(t, err)

	sorted, err := dag.TopologicalSort()
	require.NoError(t, err)
	require.Len(t, sorted, 1)
	assert.Equal(t, "a", sorted[0].Name)
}

func TestBuildDependencyGraph_LinearChain(t *testing.T) {
	// A -> B -> C (C depends on B, B depends on A)
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(
		pkg("a"),
		pkg("b", "a"),
		pkg("c", "b"),
	))
	require.NoError(t, err)

	sorted, err := dag.TopologicalSort()
	require.NoError(t, err)
	require.Len(t, sorted, 3)

	// A must come before B, B must come before C
	posA := packageIndex(sorted, "a")
	posB := packageIndex(sorted, "b")
	posC := packageIndex(sorted, "c")
	assert.Less(t, posA, posB, "a should come before b")
	assert.Less(t, posB, posC, "b should come before c")
}

func TestBuildDependencyGraph_LinearChain_Levels(t *testing.T) {
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(
		pkg("a"),
		pkg("b", "a"),
		pkg("c", "b"),
	))
	require.NoError(t, err)

	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)
	require.Len(t, levels, 3)

	assert.Len(t, levels[0], 1)
	assert.Equal(t, "a", levels[0][0].Name)

	assert.Len(t, levels[1], 1)
	assert.Equal(t, "b", levels[1][0].Name)

	assert.Len(t, levels[2], 1)
	assert.Equal(t, "c", levels[2][0].Name)
}

func TestBuildDependencyGraph_DiamondPattern(t *testing.T) {
	// A -> B, A -> C, B -> D, C -> D
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(
		pkg("a"),
		pkg("b", "a"),
		pkg("c", "a"),
		pkg("d", "b", "c"),
	))
	require.NoError(t, err)

	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)
	require.Len(t, levels, 3)

	// Level 0: A (no deps)
	assert.Len(t, levels[0], 1)
	assert.Equal(t, "a", levels[0][0].Name)

	// Level 1: B and C (both depend only on A)
	assert.Len(t, levels[1], 2)
	level1Names := packageNames(levels[1])
	assert.Contains(t, level1Names, "b")
	assert.Contains(t, level1Names, "c")

	// Level 2: D (depends on B and C)
	assert.Len(t, levels[2], 1)
	assert.Equal(t, "d", levels[2][0].Name)
}

func TestBuildDependencyGraph_WideParallel(t *testing.T) {
	// A, B, C all independent
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(
		pkg("a"),
		pkg("b"),
		pkg("c"),
	))
	require.NoError(t, err)

	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)
	require.Len(t, levels, 1)

	assert.Len(t, levels[0], 3)
	level0Names := packageNames(levels[0])
	assert.Contains(t, level0Names, "a")
	assert.Contains(t, level0Names, "b")
	assert.Contains(t, level0Names, "c")
}

func TestBuildDependencyGraph_MixedParallelAndSequential(t *testing.T) {
	// A has no deps; B depends on A; C has no deps; D depends on B and C
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(
		pkg("a"),
		pkg("b", "a"),
		pkg("c"),
		pkg("d", "b", "c"),
	))
	require.NoError(t, err)

	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)
	require.Len(t, levels, 3)

	// Level 0: A and C (no deps)
	level0Names := packageNames(levels[0])
	assert.Contains(t, level0Names, "a")
	assert.Contains(t, level0Names, "c")

	// Level 1: B (depends on A, which is in level 0)
	assert.Len(t, levels[1], 1)
	assert.Equal(t, "b", levels[1][0].Name)

	// Level 2: D (depends on B and C)
	assert.Len(t, levels[2], 1)
	assert.Equal(t, "d", levels[2][0].Name)
}

func TestBuildDependencyGraph_CycleDetection(t *testing.T) {
	// A -> B -> A (cycle)
	_, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(
		pkg("a", "b"),
		pkg("b", "a"),
	))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestBuildDependencyGraph_ThreeNodeCycle(t *testing.T) {
	// A -> B -> C -> A
	_, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(
		pkg("a", "c"),
		pkg("b", "a"),
		pkg("c", "b"),
	))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestBuildDependencyGraph_MissingDependency(t *testing.T) {
	_, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(
		pkg("a", "nonexistent"),
	))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown package")
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestBuildDependencyGraph_EmptyDependsOn(t *testing.T) {
	b := bundleWith(Package{
		Name:      "a",
		Source:    "oci://example.com/a:v1",
		DependsOn: []PackageRef{},
	})
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, b)
	require.NoError(t, err)

	sorted, err := dag.TopologicalSort()
	require.NoError(t, err)
	require.Len(t, sorted, 1)
	assert.Equal(t, "a", sorted[0].Name)
}

func TestBuildDependencyGraph_NilDependsOn(t *testing.T) {
	b := bundleWith(Package{
		Name:   "a",
		Source: "oci://example.com/a:v1",
	})
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, b)
	require.NoError(t, err)

	sorted, err := dag.TopologicalSort()
	require.NoError(t, err)
	require.Len(t, sorted, 1)
}

func TestBuildDependencyGraph_InitBundle(t *testing.T) {
	// Mirrors tests/test_data/bundles/deploy/init/bundle.uds.hcl
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(
		pkg("uds_k3d_dev"),
		pkg("init", "uds_k3d_dev"),
	))
	require.NoError(t, err)

	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)
	require.Len(t, levels, 2)

	assert.Len(t, levels[0], 1)
	assert.Equal(t, "uds_k3d_dev", levels[0][0].Name)

	assert.Len(t, levels[1], 1)
	assert.Equal(t, "init", levels[1][0].Name)
}

func TestBuildDependencyGraph_SpecCompliantBundle(t *testing.T) {
	// Mirrors tests/test_data/bundles/spec-compliant/bundle.uds.hcl
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(
		pkg("core_base"),
		pkg("core_logging", "core_base"),
		pkg("core_monitoring", "core_base", "core_logging"),
	))
	require.NoError(t, err)

	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)
	require.Len(t, levels, 3)

	assert.Len(t, levels[0], 1)
	assert.Equal(t, "core_base", levels[0][0].Name)

	assert.Len(t, levels[1], 1)
	assert.Equal(t, "core_logging", levels[1][0].Name)

	assert.Len(t, levels[2], 1)
	assert.Equal(t, "core_monitoring", levels[2][0].Name)
}

func TestDAG_Level(t *testing.T) {
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(
		pkg("a"),
		pkg("b", "a"),
		pkg("c", "b"),
	))
	require.NoError(t, err)

	assert.Equal(t, 0, dag.Level("a"))
	assert.Equal(t, 1, dag.Level("b"))
	assert.Equal(t, 2, dag.Level("c"))
	assert.Equal(t, -1, dag.Level("nonexistent"))
}

func TestDAG_Traversal(t *testing.T) {
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(pkg("my_pkg")))
	require.NoError(t, err)

	trav, ok := dag.Traversal("my_pkg")
	assert.True(t, ok)
	require.Len(t, trav, 2)

	_, ok = dag.Traversal("nonexistent")
	assert.False(t, ok)
}

func TestDAG_TraversalToName(t *testing.T) {
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundleWith(pkg("test_pkg")))
	require.NoError(t, err)

	trav, ok := dag.Traversal("test_pkg")
	require.True(t, ok)

	name := dag.traversalToName(trav)
	assert.Equal(t, "test_pkg", name)
}

func TestDAG_TraversalToName_Empty(t *testing.T) {
	dag := &DAG{}
	assert.Empty(t, dag.traversalToName(nil))
	assert.Empty(t, dag.traversalToName(hcl.Traversal{}))
	assert.Empty(t, dag.traversalToName(hcl.Traversal{hcl.TraverseRoot{Name: "package"}}))
}

// packageIndex returns the position of a package by name in the sorted slice.
func packageIndex(pkgs []*Package, name string) int {
	for i, p := range pkgs {
		if p.Name == name {
			return i
		}
	}
	return -1
}

// packageNames extracts names from a slice of packages.
func packageNames(pkgs []*Package) []string {
	names := make([]string, len(pkgs))
	for i, p := range pkgs {
		names[i] = p.Name
	}
	return names
}

// makeLevels builds a [][]*Package from a list of name groups for tests.
func makeLevels(groups ...[]string) [][]*Package {
	var levels [][]*Package
	for _, group := range groups {
		var level []*Package
		for _, name := range group {
			level = append(level, &Package{Name: name})
		}
		levels = append(levels, level)
	}
	return levels
}

func TestFilterLevels(t *testing.T) {
	// 3 levels: [core] -> [nginx] -> [podinfo]
	levels := makeLevels([]string{"core"}, []string{"nginx"}, []string{"podinfo"})

	tests := []struct {
		name        string
		filterNames []string
		// wantLevels is a flat shape: outer slice is levels, inner is package names
		wantLevels [][]string
		wantErr    string
	}{
		{
			name:        "empty filter returns all levels",
			filterNames: nil,
			wantLevels:  [][]string{{"core"}, {"nginx"}, {"podinfo"}},
		},
		{
			name:        "filter by single package",
			filterNames: []string{"nginx"},
			wantLevels:  [][]string{{"nginx"}},
		},
		{
			name:        "filter by multiple preserves topological level order",
			filterNames: []string{"podinfo", "nginx"},
			wantLevels:  [][]string{{"nginx"}, {"podinfo"}},
		},
		{
			name:        "duplicates are deduped",
			filterNames: []string{"nginx", "nginx"},
			wantLevels:  [][]string{{"nginx"}},
		},
		{
			name:        "invalid package name",
			filterNames: []string{"nginx", "nonexistent"},
			wantErr:     `package "nonexistent" is not in the bundle`,
		},
		{
			name:        "all invalid package names",
			filterNames: []string{"bogus"},
			wantErr:     `package "bogus" is not in the bundle`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FilterLevels(levels, tt.filterNames)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			got := make([][]string, 0, len(result))
			for _, level := range result {
				names := make([]string, 0, len(level))
				for _, p := range level {
					names = append(names, p.Name)
				}
				got = append(got, names)
			}
			assert.Equal(t, tt.wantLevels, got)
		})
	}
}

func TestFilterLevels_DropsEmptyLevels(t *testing.T) {
	// 3 levels with multiple packages
	levels := makeLevels([]string{"a", "b"}, []string{"c", "d"}, []string{"e"})

	// Filter only middle level package — outer levels should be dropped
	result, err := FilterLevels(levels, []string{"c"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Len(t, result[0], 1)
	assert.Equal(t, "c", result[0][0].Name)
}

func TestDeployOrder_InitBundle(t *testing.T) {
	bundle := &bundle.UDSBundle{
		Packages: []Package{
			{Name: "uds_k3d_dev", Source: "oci://ghcr.io/example/k3d:v1"},
			{Name: "init", Source: "oci://ghcr.io/example/init:v1", DependsOn: []PackageRef{{Name: "uds_k3d_dev"}}},
		},
	}

	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundle)
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
	bundle := &bundle.UDSBundle{
		Packages: []Package{
			{Name: "standalone", Source: "oci://ghcr.io/example/pkg:v1"},
		},
	}

	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundle)
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
	bundle := &bundle.UDSBundle{
		Packages: []Package{
			{Name: "pkg_c", Source: "oci://example/c:v1", DependsOn: []PackageRef{{Name: "pkg_b"}}},
			{Name: "pkg_b", Source: "oci://example/b:v1", DependsOn: []PackageRef{{Name: "pkg_a"}}},
			{Name: "pkg_a", Source: "oci://example/a:v1"},
		},
	}

	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundle)
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
	bundle := &bundle.UDSBundle{
		Packages: []Package{
			{Name: "base", Source: "oci://example/base:v1"},
			{Name: "left", Source: "oci://example/left:v1", DependsOn: []PackageRef{{Name: "base"}}},
			{Name: "right", Source: "oci://example/right:v1", DependsOn: []PackageRef{{Name: "base"}}},
			{Name: "top", Source: "oci://example/top:v1", DependsOn: []PackageRef{{Name: "left"}, {Name: "right"}}},
		},
	}

	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, bundle)
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
