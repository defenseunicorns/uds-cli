// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"fmt"
	"strings"
)

// BufferString returns a human-readable summary of the bundle as a buffer.
// Packages are displayed in deployment order (topological sort) when the
// dependency graph can be built. Falls back to declaration order on error.
func (b *UDSBundle) BufferString() (*bytes.Buffer, error) {
	var out bytes.Buffer

	if _, err := fmt.Fprint(&out, "BUNDLE METADATA\n"); err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(&out, "  Name:        %s\n", b.Metadata.Name); err != nil {
		return nil, err
	}
	if b.Metadata.Description != "" {
		if _, err := fmt.Fprintf(&out, "  Description: %s\n", b.Metadata.Description); err != nil {
			return nil, err
		}
	}
	if b.Metadata.Version != "" {
		if _, err := fmt.Fprintf(&out, "  Version:     %s\n", b.Metadata.Version); err != nil {
			return nil, err
		}
	}

	if _, err := fmt.Fprintf(&out, "\nPACKAGES (%d)\n", len(b.Packages)); err != nil {
		return nil, err
	}

	// Try to display packages in deployment order using the DAG.
	// Fall back to declaration order if the graph can't be built.
	dag, dagErr := BuildDependencyGraph(b)
	if dagErr == nil {
		if err := writePackagesInDeployOrder(&out, dag); err != nil {
			return nil, err
		}
	} else {
		if err := writePackagesFlat(&out, b.Packages); err != nil {
			return nil, err
		}
	}

	return &out, nil
}

// writePackagesInDeployOrder writes packages grouped by deployment level.
func writePackagesInDeployOrder(out *bytes.Buffer, dag *DAG) error {
	levels, err := dag.TopologicalLevels()
	if err != nil {
		return err
	}

	for levelIdx, level := range levels {
		for _, pkg := range level {
			if err := writePackageEntry(out, pkg, levelIdx); err != nil {
				return err
			}
		}
	}

	return nil
}

// writePackagesFlat writes packages in the order they appear in the slice (no level info).
func writePackagesFlat(out *bytes.Buffer, pkgs []Package) error {
	for i := range pkgs {
		if err := writePackageEntry(out, &pkgs[i], -1); err != nil {
			return err
		}
	}
	return nil
}

// writePackageEntry writes a single package entry. When level >= 0, a deploy
// level annotation is included.
func writePackageEntry(out *bytes.Buffer, pkg *Package, level int) error {
	if level >= 0 {
		if _, err := fmt.Fprintf(out, "  %s (deploy level: %d)\n", pkg.Name, level); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(out, "  %s\n", pkg.Name); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(out, "    Source: %s\n", pkg.Source); err != nil {
		return err
	}
	if pkg.Namespace != "" {
		if _, err := fmt.Fprintf(out, "    Namespace: %s\n", pkg.Namespace); err != nil {
			return err
		}
	}
	if len(pkg.DependsOn) > 0 {
		depNames := make([]string, len(pkg.DependsOn))
		for i, ref := range pkg.DependsOn {
			depNames[i] = ref.Name
		}
		if _, err := fmt.Fprintf(out, "    DependsOn: %s\n", strings.Join(depNames, ", ")); err != nil {
			return err
		}
	}
	if len(pkg.ValueFiles) > 0 {
		if _, err := fmt.Fprintf(out, "    Value Files: %s\n", strings.Join(pkg.ValueFiles, ", ")); err != nil {
			return err
		}
	}
	if _, err := out.WriteString("\n"); err != nil {
		return err
	}
	return nil
}
