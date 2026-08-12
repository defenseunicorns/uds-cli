// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
)

// helmTimeout is the timeout for Helm operations during package removal.
const helmTimeout = 15 * time.Minute

var _ Remover = (*ZarfRemover)(nil)

// deployedKey is the lookup key used in the deployed-packages cache. Zarf
// uniquely identifies a deployed package by (metadata.name, namespaceOverride):
// the same package can be deployed twice into different namespaces, producing
// distinct state secrets (zarf-package-<name> vs zarf-package-<name>-override-<ns>).
// Keying only by name would collapse those.
func deployedKey(zarfName, namespaceOverride string) string {
	return zarfName + "/" + namespaceOverride
}

// NewZarfRemover creates a new ZarfRemover. streams carries the diagnostic sink
// (streams.ErrOut, typically the command's Streams.ErrOut) used for the Zarf
// logger during removal, and the leveled logger for UDS-side diagnostics.
func NewZarfRemover(streams iostreams.IOStreams) *ZarfRemover {
	r := &ZarfRemover{streams: streams}
	r.pkgRemover = r
	return r
}

// getCluster returns the cached cluster client, creating it on first call.
func (r *ZarfRemover) getCluster(ctx context.Context) (*cluster.Cluster, error) {
	if r.cluster != nil {
		return r.cluster, nil
	}
	c, err := cluster.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to cluster: %w", err)
	}
	r.cluster = c
	return c, nil
}

// deployedPackages returns the set of deployed Zarf packages keyed by
// deployedKey(metadata.name, namespaceOverride), fetching from the cluster on
// first call and caching the result.
func (r *ZarfRemover) deployedPackages(ctx context.Context) (map[string]struct{}, error) {
	r.deployedMu.Lock()
	defer r.deployedMu.Unlock()
	if r.deployedLoaded {
		return r.deployed, nil
	}

	c, err := r.getCluster(ctx)
	if err != nil {
		return nil, err
	}

	pkgs, err := c.GetDeployedZarfPackages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployed packages: %w", err)
	}

	set := make(map[string]struct{}, len(pkgs))
	for _, p := range pkgs {
		set[deployedKey(p.Name, p.NamespaceOverride)] = struct{}{}
	}
	r.deployed = set
	r.deployedLoaded = true
	return set, nil
}

// RemoveBundle removes the bundle's packages from the cluster, calling
// RemovePackage for each package in REVERSE topological order. When packages
// is non-empty, only those package names are removed. Packages that are not
// currently deployed are skipped via the ErrPackageNotDeployed sentinel.
func (r *ZarfRemover) RemoveBundle(ctx context.Context, b *UDSBundle, packages []string, opts RemovePackageOptions) (*RemoveResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errNil("bundle")
	}
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("bundle validation failed: %w", err)
	}
	s := logger.Bind(r.streams, opts.Config.Global.LogLevel)

	dag, err := bundleinternal.BuildDependencyGraph(ctx, s, b)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	levels, err := dag.TopologicalLevels()
	if err != nil {
		return nil, fmt.Errorf("failed to compute deployment levels: %w", err)
	}
	s.Debug("dependency graph built", "levels", len(levels))

	if err := bundleinternal.ValidatePackageNames(packages, b.Packages); err != nil {
		return nil, err
	}
	if levels, err = bundleinternal.FilterLevels(levels, packages); err != nil {
		return nil, err
	}

	removed, skipped, err := r.removePackages(ctx, s, levels, opts)
	if err != nil {
		return nil, err
	}

	return &RemoveResult{
		BundleName: b.Metadata.Name,
		Removed:    removed,
		Skipped:    skipped,
	}, nil
}

// removePackages walks the DAG levels in REVERSE topological order, dispatching
// each package to the per-package primitive (r.pkgRemover.RemovePackage).
// Removal is sequential to keep teardown predictable. Packages that signal
// ErrPackageNotDeployed are counted as skipped rather than failed.
func (r *ZarfRemover) removePackages(ctx context.Context, log iostreams.IOStreams, levels [][]*Package, opts RemovePackageOptions) (removed, skipped int, err error) {
	totalPkgs := 0
	for _, level := range levels {
		totalPkgs += len(level)
	}

	pkgNum := 0
	for i := len(levels) - 1; i >= 0; i-- {
		level := levels[i]
		log.Info("starting removal level", "level", i+1, "total_levels", len(levels), "packages", len(level))

		for _, pkg := range level {
			pkgNum++
			log.Info("removing package", "name", pkg.Name, "package", pkgNum, "total", totalPkgs)

			if err := r.pkgRemover.RemovePackage(ctx, pkg, opts); err != nil {
				if errors.Is(err, ErrPackageNotDeployed) {
					log.Warn("skipping removal, package not deployed", "name", pkg.Name)
					skipped++
					continue
				}
				return removed, skipped, fmt.Errorf("failed to remove package %q: %w", pkg.Name, err)
			}
			log.Info("package removed", "name", pkg.Name)
			removed++
		}

		log.Debug("removal level complete", "level", i+1)
	}

	return removed, skipped, nil
}

// RemovePackage removes a single Zarf package from the cluster.
// Returns ErrPackageNotDeployed if the package is not present on the cluster.
//
// The bundle's pkg.Name is the HCL block label (a bundle-internal identifier
// constrained to be a valid HCL traversal name), which need not equal the
// Zarf package's metadata.name. To find the deployed package on the cluster
// we load metadata from pkg.Source and look up by (zarfMetadataName, namespace).
func (r *ZarfRemover) RemovePackage(ctx context.Context, pkg *Package, opts RemovePackageOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if pkg == nil {
		return errNil("package")
	}
	s := logger.Bind(r.streams, opts.Config.Global.LogLevel)
	s.Info("removing zarf package", "name", pkg.Name, "source", pkg.Source)

	ctx = newZarfLoggerContext(ctx, s)

	c, err := r.getCluster(ctx)
	if err != nil {
		return err
	}

	loadOpts := packager.LoadOptions{
		Architecture:   opts.Config.Options.Architecture,
		Filter:         filters.Combine(filters.ByLocalOS(runtime.GOOS)),
		OCIConcurrency: opts.Config.Options.Concurrency,
	}

	// Load metadata from pkg.Source to discover the Zarf metadata.name. For OCI
	// sources, only the metadata layer is pulled (a few KB), not the full package.
	zarfPkg, err := packager.GetPackageFromSourceOrCluster(ctx, c, pkg.Source, pkg.Namespace, loadOpts)
	if err != nil {
		return fmt.Errorf("unable to load package %q from %s: %w", pkg.Name, pkg.Source, err)
	}

	deployed, err := r.deployedPackages(ctx)
	if err != nil {
		return err
	}
	if _, ok := deployed[deployedKey(zarfPkg.Metadata.Name, pkg.Namespace)]; !ok {
		return ErrPackageNotDeployed
	}

	removeOpts := packager.RemoveOptions{
		Cluster:           c,
		Timeout:           helmTimeout,
		NamespaceOverride: pkg.Namespace,
	}

	if err := packager.Remove(ctx, zarfPkg, removeOpts); err != nil {
		return fmt.Errorf("failed to remove package %q: %w", pkg.Name, err)
	}

	return nil
}
