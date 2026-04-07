// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"fmt"
	"strings"
)

// ToInspectResult converts a UDSBundle to a serializable InspectResult.
// Packages are listed in DAG (deployment) order.
func (b *UDSBundle) ToInspectResult() (*InspectResult, error) {
	dag, err := BuildDependencyGraph(b)
	if err != nil {
		return nil, fmt.Errorf("building dependency graph: %w", err)
	}

	sorted, err := dag.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("topological sort: %w", err)
	}

	result := &InspectResult{
		Name:        b.Metadata.Name,
		Description: b.Metadata.Description,
		Version:     b.Metadata.Version,
		Packages:    make([]PackageSummary, len(sorted)),
	}
	for i, pkg := range sorted {
		result.Packages[i] = toPackageSummary(pkg)
	}
	return result, nil
}

func toPackageSummary(pkg *Package) PackageSummary {
	var depNames []string
	if len(pkg.DependsOn) > 0 {
		depNames = make([]string, len(pkg.DependsOn))
		for i, ref := range pkg.DependsOn {
			depNames[i] = ref.Name
		}
	}
	return PackageSummary{
		Name:        pkg.Name,
		Source:      pkg.Source,
		Namespace:   pkg.Namespace,
		DependsOn:   depNames,
		ValuesFiles: pkg.ValuesFiles,
	}
}

// BufferString returns a human-readable summary of the bundle as a buffer.
// Packages are displayed in deployment order (topological sort) when the
// dependency graph can be built. Falls back to declaration order on error.
func (b *UDSBundle) BufferString() *bytes.Buffer {
	var out bytes.Buffer

	fmt.Fprint(&out, "BUNDLE METADATA\n")
	fmt.Fprintf(&out, "  Name:        %s\n", b.Metadata.Name)
	if b.Metadata.Description != "" {
		fmt.Fprintf(&out, "  Description: %s\n", b.Metadata.Description)
	}
	if b.Metadata.Version != "" {
		fmt.Fprintf(&out, "  Version:     %s\n", b.Metadata.Version)
	}

	fmt.Fprintf(&out, "\nPACKAGES (%d)\n", len(b.Packages))

	// Try to display packages in deployment order using the DAG.
	// Fall back to declaration order if the graph can't be built.
	dag, dagErr := BuildDependencyGraph(b)
	if dagErr == nil {
		writePackagesInDeployOrder(&out, dag)
	} else {
		writePackagesFlat(&out, b.Packages)
	}

	return &out
}

// writePackagesInDeployOrder writes packages grouped by deployment level.
func writePackagesInDeployOrder(out *bytes.Buffer, dag *DAG) {
	levels, err := dag.TopologicalLevels()
	if err != nil {
		return
	}

	for levelIdx, level := range levels {
		for _, pkg := range level {
			writePackageEntry(out, pkg, levelIdx)
		}
	}
}

// writePackagesFlat writes packages in the order they appear in the slice (no level info).
func writePackagesFlat(out *bytes.Buffer, pkgs []Package) {
	for i := range pkgs {
		writePackageEntry(out, &pkgs[i], -1)
	}
}

// writePackageEntry writes a single package entry. When level >= 0, a deploy
// level annotation is included.
func writePackageEntry(out *bytes.Buffer, pkg *Package, level int) {
	if level >= 0 {
		fmt.Fprintf(out, "  %s (deploy level: %d)\n", pkg.Name, level)
	} else {
		fmt.Fprintf(out, "  %s\n", pkg.Name)
	}

	fmt.Fprintf(out, "    Source: %s\n", pkg.Source)
	if pkg.Namespace != "" {
		fmt.Fprintf(out, "    Namespace: %s\n", pkg.Namespace)
	}
	if len(pkg.DependsOn) > 0 {
		depNames := make([]string, len(pkg.DependsOn))
		for i, ref := range pkg.DependsOn {
			depNames[i] = ref.Name
		}
		fmt.Fprintf(out, "    DependsOn: %s\n", strings.Join(depNames, ", "))
	}
	if len(pkg.ValuesFiles) > 0 {
		fmt.Fprintf(out, "    Value Files: %s\n", strings.Join(pkg.ValuesFiles, ", "))
	}
	out.WriteString("\n")
}
