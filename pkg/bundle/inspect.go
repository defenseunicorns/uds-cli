// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/internal/bundlehcl"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// ToInspectResult converts a UDSBundle to a serializable InspectResult.
// Packages are listed in DAG (deployment) order.
func ToInspectResult(ctx context.Context, b *spec.UDSBundle, streams iostreams.IOStreams) (*InspectResult, error) {
	dag, err := bundlehcl.BuildDependencyGraph(ctx, streams, b)
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

// toPackageSummary converts a package model to its inspect representation.
func toPackageSummary(pkg *spec.Package) PackageSummary {
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
