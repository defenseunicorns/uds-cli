// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle implements UDS bundle deployment functionality.
//
// Below is an illustration of the data flow
//
//	bundle.uds.hcl ──▶ HCLParser ──▶ UDSBundle ──▶ Validate ──▶ DAG ──▶ Levels ──▶ Deploy(opts)
//	                                                              UDSBundleConfig (pre-resolved) ──┘
package bundle

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
)

// Deploy deploys a UDS bundle to a Kubernetes cluster.
// It validates the bundle, builds a dependency graph, and deploys packages
// in topological order. Config resolution is handled by the caller;
// opts.Config must be non-nil (see ValidateConfig).
//
// When opts.Bundle is set, parsing and validation of the bundle file is skipped
// (the caller is responsible for having already validated the bundle).
//
// IMPORTANT: The caller (pkg/cmd/bundle/deploy.go) is responsible for validating the bundle path
// via util.ValidateBundlePath() and resolving it via util.ResolveBundlePath() before calling this function.
func Deploy(ctx context.Context, opts DeployOptions) (*DeployResult, error) {
	if err := ValidateConfig(opts.Config); err != nil {
		return nil, err
	}

	parser := NewHCLParser()

	// Use pre-parsed bundle if provided, otherwise parse from BundlePath
	b := opts.Bundle
	if b == nil {
		slog.Debug("parsing bundle", "path", opts.BundlePath)
		var err error
		b, err = parser.ParseBundleFile(ctx, opts.BundlePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse bundle: %w", err)
		}
		slog.Debug("bundle parsed", "name", b.Metadata.Name, "packages", len(b.Packages))

		// Validate only when freshly parsed (caller is responsible for pre-parsed bundles)
		if err := b.Validate(); err != nil {
			return nil, fmt.Errorf("bundle validation failed: %w", err)
		}
		slog.Debug("bundle validated")
	}

	// Build the hcl.Traversal-based dependency graph
	dag, err := BuildDependencyGraph(b)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Get packages grouped by deployment levels (waves)
	levels, err := dag.TopologicalLevels()
	if err != nil {
		return nil, fmt.Errorf("failed to compute deployment levels: %w", err)
	}
	slog.Debug("dependency graph built", "levels", len(levels))

	// Create deployer
	deployer := NewZarfDeployer(opts.Out)

	// =========================================================================
	// DEPLOY PACKAGES BY LEVEL (prepared for future parallel execution)
	// =========================================================================
	// Deploy packages level by level. Packages within the same level have no
	// dependencies on each other and COULD be deployed in parallel.
	//
	// Current implementation: Sequential deployment within each level.
	// Future enhancement: Use goroutines + errgroup to deploy packages within
	// a level concurrently, then wait for all to complete before next level.
	//
	// Example parallel implementation (future):
	//   for _, level := range levels {
	//       g, ctx := errgroup.WithContext(ctx)
	//       for _, pkg := range level {
	//           pkg := pkg // capture for goroutine
	//           g.Go(func() error { return deployer.DeployPackage(ctx, pkg, opts) })
	//       }
	//       if err := g.Wait(); err != nil { return err }
	//   }
	bundleDir := filepath.Dir(opts.BundlePath)
	pkgOpts := DeployPackageOptions{
		Config:    opts.Config,
		BundleDir: bundleDir,
		Prompt:    opts.Prompt,
	}

	totalPkgs := 0
	for _, level := range levels {
		totalPkgs += len(level)
	}

	pkgNum := 0
	for levelIdx, level := range levels {
		slog.Info("starting deployment level", "level", levelIdx+1, "total_levels", len(levels), "packages", len(level))

		for _, pkg := range level {
			pkgNum++
			slog.Info("deploying package", "name", pkg.Name, "source", pkg.Source, "package", pkgNum, "total", totalPkgs)

			if err := deployer.DeployPackage(ctx, pkg, pkgOpts); err != nil {
				// Use DAG's GetTraversal for enhanced error with source location
				if trav, ok := dag.GetTraversal(pkg.Name); ok {
					srcRange := trav.SourceRange()
					return nil, fmt.Errorf("failed to deploy package %q at %s: %w",
						pkg.Name, srcRange.String(), err)
				}
				return nil, fmt.Errorf("failed to deploy package %q: %w", pkg.Name, err)
			}

			slog.Info("package deployed", "name", pkg.Name)
		}

		slog.Debug("deployment level complete", "level", levelIdx+1)
	}
	// =========================================================================

	return &DeployResult{
		BundleName: b.Metadata.Name,
		Packages:   len(b.Packages),
	}, nil
}
