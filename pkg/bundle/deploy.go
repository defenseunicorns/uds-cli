// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle implements UDS bundle deployment functionality.
//
// Below is an illustration of the data flow
//
//	bundle.uds.hcl ──▶ HCLParser ──▶ UDSBundle ──▶ Validate ──▶ DAG ──▶ Levels ──▶ Deploy
//	     │                              │                        │          │
//	     │                              │                        │          │
//	   locals                      []Package                  detect     [level0]
//	   resolved                   + []PackageRef              cycles     [level1]
//	   via EvalContext                                                   [level2]
package bundle

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
)

// Deploy deploys a UDS bundle to a Kubernetes cluster.
// It uses the hcl.Traversal-based DAG from Milestone 3 to determine deployment order.
//
// IMPORTANT: The caller (pkg/cmd/bundle/deploy.go) is responsible for validating the bundle path
// via util.ValidateBundlePath() and resolving it via util.ResolveBundlePath() before calling this function.
// This function receives an already-validated path to a bundle.uds.hcl file. OCI reference detection,
// tar.zst rejection, path existence checks, and directory resolution are all handled by the
// shared util functions at the command layer.
func Deploy(ctx context.Context, opts DeployOptions) error {
	// Parse the bundle (path already validated by command layer via util.ValidateBundlePath)
	slog.Debug("parsing bundle", "path", opts.BundlePath)
	bundle, err := NewHCLParser().ParseBundleFile(ctx, opts.BundlePath)
	if err != nil {
		return fmt.Errorf("failed to parse bundle: %w", err)
	}
	slog.Debug("bundle parsed", "name", bundle.Metadata.Name, "packages", len(bundle.Packages))

	// Validate the bundle
	if err := bundle.Validate(); err != nil {
		return fmt.Errorf("bundle validation failed: %w", err)
	}
	slog.Debug("bundle validation passed")

	// Build the hcl.Traversal-based dependency graph
	dag, err := BuildDependencyGraph(bundle)
	if err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Get packages grouped by deployment levels (waves)
	levels, err := dag.TopologicalLevels()
	if err != nil {
		return fmt.Errorf("failed to compute deployment levels: %w", err)
	}
	slog.Debug("dependency graph built", "levels", len(levels))

	// Create deployer
	deployer := NewZarfDeployer(opts.TmpDir, opts.Out)

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
		RegistryOptions: opts.RegistryOptions,
		BundleDir:       bundleDir,
		Prompt:          opts.Prompt,
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
					return fmt.Errorf("failed to deploy package %q at %s: %w",
						pkg.Name, srcRange.String(), err)
				}
				return fmt.Errorf("failed to deploy package %q: %w", pkg.Name, err)
			}

			slog.Info("package deployed", "name", pkg.Name)
		}

		slog.Debug("deployment level complete", "level", levelIdx+1)
	}
	// =========================================================================

	return nil
}
