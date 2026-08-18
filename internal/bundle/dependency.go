// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"sort"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/hashicorp/hcl/v2"
)

// BuildDependencyGraph constructs a DAG from bundle packages using hcl.Traversal.
// Each package is represented as a traversal "package.<name>", and dependencies
// are taken from the already-parsed PackageRef values in the Package struct.
// The graph is validated for missing references and cycles before being returned.
func BuildDependencyGraph(ctx context.Context, streams iostreams.IOStreams, bundle *spec.UDSBundle) (*DAG, error) {
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
				return nil, fmt.Errorf("package %q depends on unknown package %q", pt.Package.Name, ref.Name)
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

	streams.Debug("dependency graph constructed", "packages", len(packages), "edges", len(edges))
	return dag, nil
}

// TopologicalSort returns packages in deployment order (dependencies first).
// It flattens the levels from TopologicalLevels into a single slice.
func (d *DAG) TopologicalSort() ([]*spec.Package, error) {
	levels, err := d.TopologicalLevels()
	if err != nil {
		return nil, err
	}

	var sorted []*spec.Package
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
func (d *DAG) TopologicalLevels() ([][]*spec.Package, error) {
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
	var levels [][]*spec.Package

	for remaining > 0 {
		var currentLevel []*spec.Package
		for name, degree := range inDegree {
			if degree == 0 {
				currentLevel = append(currentLevel, d.packages[name].Package)
			}
		}

		if len(currentLevel) == 0 {
			return nil, fmt.Errorf("cycle detected in package dependencies")
		}

		// Sort packages within a level by name for deterministic output.
		sort.Slice(currentLevel, func(i, j int) bool {
			return currentLevel[i].Name < currentLevel[j].Name
		})

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

// FilterLevels keeps only packages whose names appear in filterNames, preserving
// the topological level structure. When filterNames is empty, all levels are
// returned unchanged. Empty levels (after filtering) are dropped. Duplicate
// names are deduped. Returns an error if any requested name is not in the bundle.
func FilterLevels(levels [][]*spec.Package, filterNames []string) ([][]*spec.Package, error) {
	if len(filterNames) == 0 {
		return levels, nil
	}

	requested := make(map[string]bool, len(filterNames))
	for _, name := range filterNames {
		requested[name] = true
	}

	bundleNames := make(map[string]bool)
	for _, level := range levels {
		for _, pkg := range level {
			bundleNames[pkg.Name] = true
		}
	}
	for name := range requested {
		if !bundleNames[name] {
			return nil, fmt.Errorf("package %q is not in the bundle", name)
		}
	}

	var filtered [][]*spec.Package
	for _, level := range levels {
		var kept []*spec.Package
		for _, pkg := range level {
			if requested[pkg.Name] {
				kept = append(kept, pkg)
			}
		}
		if len(kept) > 0 {
			filtered = append(filtered, kept)
		}
	}
	return filtered, nil
}

// Level returns the deployment level (wave) for a specific package.
// Level 0 packages have no dependencies, level 1 depends only on level 0, etc.
// Returns -1 if the package is not found or if levels cannot be computed.
func (d *DAG) Level(name string) int {
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

// Traversal returns the hcl.Traversal for a package by name.
// This can be used for enhanced error messages with HCL source locations.
func (d *DAG) Traversal(name string) (hcl.Traversal, bool) {
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

// ValidatePackageNames checks that all names exist in the bundle's package list.
// The error message names the unknown packages and lists all available packages.
func ValidatePackageNames(names []string, packages []spec.Package) error {
	if len(names) == 0 {
		return nil
	}
	known := make(map[string]bool, len(packages))
	knownNames := make([]string, 0, len(packages))
	for _, p := range packages {
		known[p.Name] = true
		knownNames = append(knownNames, p.Name)
	}
	sort.Strings(knownNames)
	var unknown []string
	for _, n := range names {
		if !known[n] {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown packages %v not defined in bundle (available packages: %v)", unknown, knownNames)
	}
	return nil
}

// RemovalViolations returns packages that would retain a dependency on each
// selected package after removal. An empty result means the removal is safe.
func RemovalViolations(ctx context.Context, streams iostreams.IOStreams, b *spec.UDSBundle, packageNames []string) (map[string][]string, error) {
	if len(packageNames) == 0 {
		return nil, nil
	}
	dag, err := BuildDependencyGraph(ctx, streams, b)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}
	return dependentBlockers(dag, packageNames), nil
}

// DeployViolations returns each selected package's dependencies that are not
// selected. An empty result means the deployment is safe.
func DeployViolations(ctx context.Context, streams iostreams.IOStreams, b *spec.UDSBundle, packageNames []string) (map[string][]string, error) {
	if len(packageNames) == 0 {
		return nil, nil
	}
	dag, err := BuildDependencyGraph(ctx, streams, b)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}
	return missingDependencies(dag, packageNames), nil
}

// dependentBlockers returns, for each package being removed, the names of
// other bundle packages that depend on it but are NOT themselves being removed.
// Only direct dependents are reported; transitive impacts surface through the
// chain (if B depends on A and C depends on B, removing A flags B; the user can
// re-run after deciding what to do about B).
func dependentBlockers(dag *DAG, removeNames []string) map[string][]string {
	removeSet := make(map[string]bool, len(removeNames))
	for _, n := range removeNames {
		removeSet[n] = true
	}

	blockers := make(map[string][]string)
	for name, deps := range dag.edges {
		if removeSet[name] {
			continue
		}
		for _, trav := range deps {
			depName := dag.traversalToName(trav)
			if removeSet[depName] {
				blockers[depName] = append(blockers[depName], name)
			}
		}
	}

	for k := range blockers {
		sort.Strings(blockers[k])
	}
	return blockers
}

// missingDependencies returns, for each selected package, the names of its
// direct dependencies that are NOT in the selected set.
func missingDependencies(dag *DAG, selected []string) map[string][]string {
	selSet := make(map[string]bool, len(selected))
	for _, n := range selected {
		selSet[n] = true
	}

	missing := make(map[string][]string)
	for name, deps := range dag.edges {
		if !selSet[name] {
			continue
		}
		for _, trav := range deps {
			depName := dag.traversalToName(trav)
			if !selSet[depName] {
				missing[name] = append(missing[name], depName)
			}
		}
	}

	for k := range missing {
		sort.Strings(missing[k])
	}
	return missing
}
