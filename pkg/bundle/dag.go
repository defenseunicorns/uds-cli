// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
	"log/slog"

	"github.com/hashicorp/hcl/v2"
)

// PackageTraversal wraps a package with an hcl.Traversal for structured
// reference handling. The bundle HCL syntax uses expression-based dependencies
// (depends_on = [package.core_base]), which are parsed into hcl.Traversal values
// for type safety and source location tracking.
// Inspired by OpenTofu's dependency handling:
// https://github.com/opentofu/opentofu/blob/d088667ed7e5bd611ce8d922add3f5ec313beae1/internal/configs/depends_on.go#L13
type PackageTraversal struct {
	Package   *Package
	Traversal hcl.Traversal
}

// DAG represents a directed acyclic graph of package dependencies using hcl.Traversal.
// Edges point from a package to its dependencies (i.e., the things it depends on).
type DAG struct {
	packages map[string]*PackageTraversal
	edges    map[string][]hcl.Traversal // package name -> dependency traversals
}

// BuildDependencyGraph constructs a DAG from bundle packages using hcl.Traversal.
// Each package is represented as a traversal "package.<name>", and dependencies
// are taken from the already-parsed PackageRef values in the Package struct.
// The graph is validated for missing references and cycles before being returned.
func BuildDependencyGraph(bundle *UDSBundle) (*DAG, error) {
	packages := make(map[string]*PackageTraversal, len(bundle.Packages))

	for i := range bundle.Packages {
		pkg := &bundle.Packages[i]

		traversal := hcl.Traversal{
			hcl.TraverseRoot{Name: "package"},
			hcl.TraverseAttr{Name: pkg.Name},
		}

		packages[pkg.Name] = &PackageTraversal{
			Package:   pkg,
			Traversal: traversal,
		}
	}

	edges := make(map[string][]hcl.Traversal, len(packages))
	for _, pt := range packages {
		var depTraversals []hcl.Traversal
		for _, ref := range pt.Package.DependsOn {
			// Use the traversal from the parsed PackageRef
			depPkg, exists := packages[ref.Name]
			if !exists {
				// Include source location in error message
				return nil, fmt.Errorf("package %q depends on unknown package %q at %s",
					pt.Package.Name, ref.Name, ref.Traversal.SourceRange())
			}
			depTraversals = append(depTraversals, depPkg.Traversal)
		}
		edges[pt.Package.Name] = depTraversals
	}

	dag := &DAG{
		packages: packages,
		edges:    edges,
	}

	if err := dag.detectCycles(); err != nil {
		return nil, err
	}

	slog.Debug("dependency graph constructed", "packages", len(packages), "edges", len(edges))
	return dag, nil
}

// TopologicalSort returns packages in deployment order (dependencies first).
// It flattens the levels from TopologicalLevels into a single slice.
func (d *DAG) TopologicalSort() ([]*Package, error) {
	levels, err := d.TopologicalLevels()
	if err != nil {
		return nil, err
	}

	var sorted []*Package
	for _, level := range levels {
		sorted = append(sorted, level...)
	}

	return sorted, nil
}

// TopologicalLevels returns packages grouped by deployment "waves" or "levels".
// Packages within the same level have no dependencies on each other and CAN be
// deployed in parallel. Levels must be deployed sequentially (level 0 before level 1, etc.).
//
// Uses Kahn's algorithm with level tracking. In-degree counts the number of
// unmet dependencies for each package. Packages with in-degree 0 form the current
// level and are "removed" from the graph by decrementing dependents' in-degrees.
//
// Example for diamond pattern (A has no deps, B and C depend on A, D depends on B and C):
//
//	Level 0: [A]        - deploy first, no dependencies
//	Level 1: [B, C]     - can deploy B and C in parallel after A completes
//	Level 2: [D]        - deploy after both B and C complete
func (d *DAG) TopologicalLevels() ([][]*Package, error) {
	// Calculate in-degree for each package (number of dependencies)
	inDegree := make(map[string]int, len(d.packages))
	for name := range d.packages {
		inDegree[name] = len(d.edges[name])
	}

	// Build reverse map: package name -> names of packages that depend on it
	dependents := make(map[string][]string)
	for name, depTraversals := range d.edges {
		for _, trav := range depTraversals {
			depName := d.traversalToName(trav)
			dependents[depName] = append(dependents[depName], name)
		}
	}

	remaining := len(d.packages)
	var levels [][]*Package

	for remaining > 0 {
		var currentLevel []*Package
		for name, degree := range inDegree {
			if degree == 0 {
				currentLevel = append(currentLevel, d.packages[name].Package)
			}
		}

		if len(currentLevel) == 0 {
			return nil, fmt.Errorf("cycle detected in package dependencies")
		}

		levels = append(levels, currentLevel)
		remaining -= len(currentLevel)

		// Remove processed packages and update dependents' in-degrees
		for _, pkg := range currentLevel {
			// Mark as processed by setting in-degree to -1
			inDegree[pkg.Name] = -1

			for _, dependent := range dependents[pkg.Name] {
				if inDegree[dependent] > 0 {
					inDegree[dependent]--
				}
			}
		}
	}

	return levels, nil
}

// GetLevel returns the deployment level (wave) for a specific package.
// Level 0 packages have no dependencies, level 1 depends only on level 0, etc.
// Returns -1 if the package is not found or if levels cannot be computed.
func (d *DAG) GetLevel(name string) int {
	levels, err := d.TopologicalLevels()
	if err != nil {
		return -1
	}

	for levelIdx, level := range levels {
		for _, pkg := range level {
			if pkg.Name == name {
				return levelIdx
			}
		}
	}
	return -1
}

// GetTraversal returns the hcl.Traversal for a package by name.
// This can be used for enhanced error messages with HCL source locations.
func (d *DAG) GetTraversal(name string) (hcl.Traversal, bool) {
	pt, exists := d.packages[name]
	if !exists {
		return nil, false
	}
	return pt.Traversal, true
}

// traversalToName extracts the package name from a traversal (package.<name>).
func (d *DAG) traversalToName(trav hcl.Traversal) string {
	if len(trav) >= 2 {
		if attr, ok := trav[1].(hcl.TraverseAttr); ok {
			return attr.Name
		}
	}
	return ""
}

// detectCycles uses DFS with a recursion stack to detect cycles in the dependency graph.
func (d *DAG) detectCycles() error {
	visited := make(map[string]bool, len(d.packages))
	recStack := make(map[string]bool, len(d.packages))

	var dfs func(name string) error
	dfs = func(name string) error {
		visited[name] = true
		recStack[name] = true

		for _, depTrav := range d.edges[name] {
			depName := d.traversalToName(depTrav)
			if !visited[depName] {
				if err := dfs(depName); err != nil {
					return err
				}
			} else if recStack[depName] {
				return fmt.Errorf("dependency cycle detected: %s -> %s", name, depName)
			}
		}

		recStack[name] = false
		return nil
	}

	for name := range d.packages {
		if !visited[name] {
			if err := dfs(name); err != nil {
				return err
			}
		}
	}

	return nil
}
