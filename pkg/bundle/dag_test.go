// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to build a UDSBundle from a slice of packages.
func bundleWith(pkgs ...Package) *UDSBundle {
	return &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "test"},
		Packages: pkgs,
	}
}

// pkgRef creates a PackageRef from a dependency name.
// This simulates what the HCL parser produces from depends_on = [package.name].
func pkgRef(name string) PackageRef {
	return PackageRef{
		Name: name,
		Traversal: hcl.Traversal{
			hcl.TraverseRoot{Name: "package"},
			hcl.TraverseAttr{Name: name},
		},
	}
}

// pkgRefs creates a slice of PackageRefs from dependency names.
func pkgRefs(names ...string) []PackageRef {
	refs := make([]PackageRef, len(names))
	for i, name := range names {
		refs[i] = pkgRef(name)
	}
	return refs
}

func pkg(name string, deps ...string) Package {
	return Package{Name: name, Source: "oci://example.com/" + name + ":v1", DependsOn: pkgRefs(deps...)}
}

func TestBuildDependencyGraph_SinglePackage(t *testing.T) {
	dag, err := BuildDependencyGraph(bundleWith(pkg("a")))
	require.NoError(t, err)

	sorted, err := dag.TopologicalSort()
	require.NoError(t, err)
	require.Len(t, sorted, 1)
	assert.Equal(t, "a", sorted[0].Name)
}

func TestBuildDependencyGraph_LinearChain(t *testing.T) {
	// A -> B -> C (C depends on B, B depends on A)
	dag, err := BuildDependencyGraph(bundleWith(
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
	dag, err := BuildDependencyGraph(bundleWith(
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
	dag, err := BuildDependencyGraph(bundleWith(
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
	dag, err := BuildDependencyGraph(bundleWith(
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
	dag, err := BuildDependencyGraph(bundleWith(
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
	_, err := BuildDependencyGraph(bundleWith(
		pkg("a", "b"),
		pkg("b", "a"),
	))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestBuildDependencyGraph_ThreeNodeCycle(t *testing.T) {
	// A -> B -> C -> A
	_, err := BuildDependencyGraph(bundleWith(
		pkg("a", "c"),
		pkg("b", "a"),
		pkg("c", "b"),
	))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestBuildDependencyGraph_MissingDependency(t *testing.T) {
	_, err := BuildDependencyGraph(bundleWith(
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
	dag, err := BuildDependencyGraph(b)
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
	dag, err := BuildDependencyGraph(b)
	require.NoError(t, err)

	sorted, err := dag.TopologicalSort()
	require.NoError(t, err)
	require.Len(t, sorted, 1)
}

func TestBuildDependencyGraph_InitBundle(t *testing.T) {
	// Mirrors tests/test_data/bundles/deploy/init/bundle.uds.hcl
	dag, err := BuildDependencyGraph(bundleWith(
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
	dag, err := BuildDependencyGraph(bundleWith(
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
	dag, err := BuildDependencyGraph(bundleWith(
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
	dag, err := BuildDependencyGraph(bundleWith(pkg("my_pkg")))
	require.NoError(t, err)

	trav, ok := dag.Traversal("my_pkg")
	assert.True(t, ok)
	require.Len(t, trav, 2)

	_, ok = dag.Traversal("nonexistent")
	assert.False(t, ok)
}

func TestDAG_TraversalToName(t *testing.T) {
	dag, err := BuildDependencyGraph(bundleWith(pkg("test_pkg")))
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
