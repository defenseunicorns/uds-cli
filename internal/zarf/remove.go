// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
)

// RemoveResult represents the result of removing a bundle.
type RemoveResult struct {
	BundleName string
	Packages   []RemovePackageResult
}

// RemovePackageResult reports the outcome for one package.
type RemovePackageResult struct {
	Name   string
	Status RemovePackageStatus
}

// RemovePackageStatus identifies a package removal outcome.
type RemovePackageStatus string

const (
	RemovePackageStatusRemoved RemovePackageStatus = "removed"
	RemovePackageStatusSkipped RemovePackageStatus = "skipped"
)

// Remover removes individual packages or complete bundles.
type Remover interface {
	// RemovePackage removes one package from a target.
	RemovePackage(context.Context, *spec.Package, RemovePackageOptions) error
	// RemoveBundle removes selected packages in reverse dependency order.
	RemoveBundle(context.Context, *spec.UDSBundle, []string, RemovePackageOptions) (*RemoveResult, error)
}

// RemovePackageOptions contains options for removing one package.
type RemovePackageOptions struct {
	// Config is the resolved removal configuration.
	Config *UDSBundleConfig
	// DeployedPackageNames maps bundle package labels to Zarf metadata names.
	DeployedPackageNames map[string]string
	// Force bypasses dependency-safety validation.
	Force bool
}
type packageRemover interface {
	RemovePackage(context.Context, *spec.Package, RemovePackageOptions) error
}

// ZarfRemover implements Remover using the Zarf Go library.
type ZarfRemover struct {
	streams        iostreams.IOStreams
	cluster        *cluster.Cluster
	deployedMu     sync.Mutex
	deployed       map[string]struct{}
	deployedLoaded bool
	pkgRemover     packageRemover
}

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

func removalPackageSource(pkg *spec.Package, deployedPackageNames map[string]string) string {
	if deployedName := deployedPackageNames[pkg.Name]; deployedName != "" {
		return deployedName
	}
	return pkg.Source
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
		return nil, fmt.Errorf("%w for package removal: %w", ErrConnectCluster, err)
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
		return nil, fmt.Errorf("%w from cluster: %w", ErrReadDeployedPackages, err)
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
func (r *ZarfRemover) RemoveBundle(ctx context.Context, b *spec.UDSBundle, packages []string, opts RemovePackageOptions) (*RemoveResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if b == nil {
		return nil, NilParameterError{Name: "bundle"}
	}
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("%w for bundle %q: %w", ErrBundleValidation, b.Metadata.Name, err)
	}
	s := logger.Bind(r.streams, opts.Config.Options.LogLevel)

	dag, err := bundleinternal.BuildDependencyGraph(ctx, s, b)
	if err != nil {
		return nil, fmt.Errorf("%w for bundle %q: %w", ErrBuildDependencyGraph, b.Metadata.Name, err)
	}

	levels, err := dag.TopologicalLevels()
	if err != nil {
		return nil, fmt.Errorf("%w for bundle %q: %w", ErrComputeDeploymentLevels, b.Metadata.Name, err)
	}
	s.Debug("dependency graph built", "levels", len(levels))

	if err := bundleinternal.ValidatePackageNames(packages, b.Packages); err != nil {
		return nil, err
	}
	if levels, err = bundleinternal.FilterLevels(levels, packages); err != nil {
		return nil, err
	}

	results, err := r.removePackages(ctx, s, levels, opts)
	if err != nil {
		return nil, err
	}

	return &RemoveResult{
		BundleName: b.Metadata.Name,
		Packages:   results,
	}, nil
}

// removePackages walks the DAG levels in REVERSE topological order, dispatching
// each package to the per-package primitive (r.pkgRemover.RemovePackage).
// Removal is sequential to keep teardown predictable. Packages that signal
// ErrPackageNotDeployed are counted as skipped rather than failed.
func (r *ZarfRemover) removePackages(ctx context.Context, log iostreams.IOStreams, levels [][]*spec.Package, opts RemovePackageOptions) ([]RemovePackageResult, error) {
	totalPkgs := 0
	for _, level := range levels {
		totalPkgs += len(level)
	}
	results := make([]RemovePackageResult, 0, totalPkgs)

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
					results = append(results, RemovePackageResult{Name: pkg.Name, Status: RemovePackageStatusSkipped})
					continue
				}
				return results, fmt.Errorf("failed to remove package %q: %w: %w", pkg.Name, ErrRemovePackage, err)
			}
			log.Info("package removed", "name", pkg.Name)
			results = append(results, RemovePackageResult{Name: pkg.Name, Status: RemovePackageStatusRemoved})
		}

		log.Debug("removal level complete", "level", i+1)
	}

	return results, nil
}

// RemovePackage removes a single Zarf package from the cluster.
// Returns ErrPackageNotDeployed if the package is not present on the cluster.
//
// The bundle's pkg.Name is the HCL block label and may differ from the Zarf
// metadata.name. Artifact-backed removal supplies the embedded Zarf name;
// source removal discovers it from pkg.Source.
func (r *ZarfRemover) RemovePackage(ctx context.Context, pkg *spec.Package, opts RemovePackageOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if pkg == nil {
		return NilParameterError{Name: "package"}
	}
	packageSource := removalPackageSource(pkg, opts.DeployedPackageNames)
	s := logger.Bind(r.streams, opts.Config.Options.LogLevel)
	s.Debug("preparing package removal", "name", pkg.Name, "source", packageSource)

	ctx = newZarfLoggerContext(ctx, s)

	deployed, err := r.deployedPackages(ctx)
	if err != nil {
		return err
	}

	// Artifact-backed removal already knows the embedded Zarf metadata.name.
	// Check it before loading cluster state so an absent package is reported as
	// skipped instead of failing the load of its nonexistent state secret.
	deployedName := opts.DeployedPackageNames[pkg.Name]
	if deployedName != "" {
		if _, ok := deployed[deployedKey(deployedName, pkg.Namespace)]; !ok {
			return ErrPackageNotDeployed
		}
	}

	c, err := r.getCluster(ctx)
	if err != nil {
		return err
	}

	loadOpts := packager.LoadOptions{
		Architecture:   opts.Config.Options.Architecture,
		Filter:         filters.Combine(filters.ByLocalOS(runtime.GOOS)),
		OCIConcurrency: opts.Config.Options.Concurrency,
	}

	// Artifact-backed removal supplies the deployed Zarf name so this loads
	// package state from the cluster. Source removal discovers the name from the
	// author-provided package source; OCI sources fetch only the metadata layer.
	zarfPkg, err := packager.GetPackageFromSourceOrCluster(ctx, c, packageSource, pkg.Namespace, loadOpts)
	if err != nil {
		return fmt.Errorf("package %q from %s: %w: %w", pkg.Name, packageSource, ErrLoadPackage, err)
	}

	deployed, err := r.deployedPackages(ctx)
	if err != nil {
		return err
	}
	if _, ok := deployed[deployedKey(zarfPkg.AsV1alpha1().Metadata.Name, pkg.Namespace)]; !ok {
		return ErrPackageNotDeployed
	}

	removeOpts := packager.RemoveOptions{
		Cluster:           c,
		Timeout:           helmTimeout,
		NamespaceOverride: pkg.Namespace,
	}

	if err := packager.Remove(ctx, zarfPkg, removeOpts); err != nil {
		return fmt.Errorf("package %q: %w: %w", pkg.Name, ErrRemovePackage, err)
	}

	return nil
}
