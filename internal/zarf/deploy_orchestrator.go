// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"fmt"
	"os"
	"sync"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"golang.org/x/sync/errgroup"
)

type orchestratedDeployer struct {
	base            *ZarfDeployer
	packageDeployFn func(context.Context, *spec.Package, DeployPackageOptions) error
}
type deployOrchestrator struct {
	deployer    packageDeployer
	dag         *bundleinternal.DAG
	levels      [][]*spec.Package
	concurrency int
	pkgOpts     DeployPackageOptions
	streams     iostreams.IOStreams
	deployedMu  sync.Mutex
	deployed    map[string]struct{}
}

type packageDeployer interface {
	DeployPackage(context.Context, *spec.Package, DeployPackageOptions) error
}

// newDeployOrchestrator wires the orchestrator with everything it needs to
// drive a single bundle deploy. Each package is deployed via deployer.DeployPackage;
// every deploy detail is carried in pkgOpts (e.g. pkgOpts.ClusterDeployFn).
func newDeployOrchestrator(deployer packageDeployer, dag *bundleinternal.DAG, levels [][]*spec.Package, concurrency int, pkgOpts DeployPackageOptions, streams iostreams.IOStreams) *deployOrchestrator {
	return &deployOrchestrator{
		deployer:    deployer,
		dag:         dag,
		levels:      levels,
		concurrency: concurrency,
		pkgOpts:     pkgOpts,
		streams:     streams,
		deployed:    make(map[string]struct{}),
	}
}

// DeployedPackages returns successfully deployed package names in DAG order.
func (o *deployOrchestrator) DeployedPackages() []string {
	o.deployedMu.Lock()
	defer o.deployedMu.Unlock()

	result := make([]string, 0, len(o.deployed))
	for _, level := range o.levels {
		for _, pkg := range level {
			if _, ok := o.deployed[pkg.Name]; ok {
				result = append(result, pkg.Name)
			}
		}
	}
	return result
}

// Run executes the deploy across all levels and returns every per-package
// failure joined via errors.Join. Deploys within a level run in parallel;
// level N+1 only starts after level N finishes. On failure, scheduling stops;
// in-flight deploys finish.
func (o *deployOrchestrator) Run(ctx context.Context) error {
	for levelIdx, level := range o.levels {
		// Honor parent ctx cancellation at the level boundary. Without this
		// check, a pre-cancelled ctx (or one cancelled between levels) would
		// silently return nil: the inner loop's gctx.Err() check would break
		// before any goroutine starts, g.Wait() would return nil with no
		// goroutines registered, and we would advance to the next level.
		if err := ctx.Err(); err != nil {
			return err
		}

		o.streams.Info("starting deployment level", "level", levelIdx+1, "total_levels", len(o.levels), "packages", len(level))

		// gctx is errgroup's derived context — cancelled when ANY goroutine
		// returns an error or the parent ctx is cancelled. We only consult
		// gctx in the scheduling loop below to decide whether to admit new
		// packages; in-flight deploy calls receive the parent ctx so a
		// sibling's failure does not abort an already-running deploy.
		// User cancellation (Ctrl+C) still propagates because the parent ctx
		// is cancelled directly. We ignore g.Wait's return and surface every
		// per-package failure via the levelErrs accumulator below: g.Wait
		// would otherwise discard all but the first sibling failure.
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(o.concurrency)

		levelErrs := newErrorAccumulator()

		for i, pkg := range level {
			if gctx.Err() != nil {
				o.streams.Warn("skipping remaining packages in level after another package failure",
					"level", levelIdx+1,
					"skipped", len(level)-i)
				break
			}
			g.Go(func() error {
				// g.Go blocks when SetLimit is reached, so this goroutine may
				// have been queued behind a now-failed sibling. Re-check after
				// the slot is acquired and skip the deploy entirely if so —
				// only packages whose deploy call has actually begun should
				// be allowed to finish. This is a cancellation, not a deploy
				// failure, so it must not be recorded in levelErrs.
				if err := gctx.Err(); err != nil {
					return err
				}

				o.streams.Info("deploying package", "name", pkg.Name, "source", pkg.Source)

				if err := o.deployer.DeployPackage(ctx, pkg, o.pkgOpts); err != nil {
					wrapped := fmt.Errorf("%s: %w", o.packageDeployFailurePrefix(pkg), err)
					levelErrs.Add(wrapped)
					// Returning the error makes errgroup cancel gctx so queued
					// siblings short-circuit; we drop g.Wait's return on the
					// floor in favour of the joined levelErrs.
					return wrapped
				}

				o.streams.Info("package deployed", "name", pkg.Name)
				o.deployedMu.Lock()
				o.deployed[pkg.Name] = struct{}{}
				o.deployedMu.Unlock()
				return nil
			})
		}

		_ = g.Wait()
		if err := levelErrs.Err(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		o.streams.Debug("deployment level complete", "level", levelIdx+1)
	}

	return nil
}

func (o *deployOrchestrator) packageDeployFailurePrefix(pkg *spec.Package) string {
	prefix := fmt.Sprintf("failed to deploy package %q", pkg.Name)
	sourceRange, ok := packageSourceRange(o.pkgOpts.bundlePath, pkg.Name)
	if !ok {
		return prefix
	}
	return fmt.Sprintf("%s at %s", prefix, sourceRange.String())
}

func packageSourceRange(bundlePath, packageName string) (hcl.Range, bool) {
	if bundlePath == "" {
		return hcl.Range{}, false
	}
	src, err := os.ReadFile(bundlePath)
	if err != nil {
		return hcl.Range{}, false
	}
	file, diags := hclsyntax.ParseConfig(src, bundlePath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return hcl.Range{}, false
	}
	content, _, diags := file.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "package", LabelNames: []string{"name"}}}})
	if diags.HasErrors() {
		return hcl.Range{}, false
	}
	for _, block := range content.Blocks {
		if len(block.Labels) == 1 && block.Labels[0] == packageName {
			return block.DefRange, true
		}
	}
	return hcl.Range{}, false
}
