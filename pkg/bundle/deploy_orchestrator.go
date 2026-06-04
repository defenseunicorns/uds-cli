// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"golang.org/x/sync/errgroup"
)

// deployPackageFunc is the per-package deploy callable consumed by
// deployOrchestrator. It exists as a function type — rather than the public
// Deployer interface or a private parallel one — purely as a test seam:
// production passes ZarfDeployer.DeployPackage as a method value; unit tests
// pass a mock function with the same signature.
//
// This lets the scheduling logic (errgroup wiring, concurrency limits,
// graceful stop on sibling failure, level barriers) be exercised in isolation
// without spinning up a real Zarf/Helm/Kubernetes stack and without forcing
// ZarfDeployer to carry a mockable struct field.
type deployPackageFunc func(ctx context.Context, pkg *Package, opts DeployPackageOptions) error

// deployOrchestrator schedules per-package deploys across a bundle's
// topological levels — parallelising within a level (bounded by concurrency),
// serialising across levels, and stopping gracefully on failure (already
// in-flight packages run to completion; queued packages are dropped).
//
// The orchestrator owns only the scheduling and error-aggregation concerns.
// Building the DAG, computing levels, and wrapping the result into a
// DeployResult are the caller's responsibility (see ZarfDeployer.DeployBundle).
type deployOrchestrator struct {
	deploy      deployPackageFunc
	dag         *DAG
	levels      [][]*Package
	concurrency int
	pkgOpts     DeployPackageOptions
	streams     iostreams.IOStreams
}

// newDeployOrchestrator wires the orchestrator with everything it needs to
// drive a single bundle deploy.
func newDeployOrchestrator(deploy deployPackageFunc, dag *DAG, levels [][]*Package, concurrency int, pkgOpts DeployPackageOptions, streams iostreams.IOStreams) *deployOrchestrator {
	return &deployOrchestrator{
		deploy:      deploy,
		dag:         dag,
		levels:      levels,
		concurrency: concurrency,
		pkgOpts:     pkgOpts,
		streams:     streams,
	}
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

		levelErrs := newErrs()

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

				if err := o.deploy(ctx, pkg, o.pkgOpts); err != nil {
					wrapped := fmt.Errorf("failed to deploy package %q: %w", pkg.Name, err)
					if o.dag != nil {
						if trav, ok := o.dag.Traversal(pkg.Name); ok {
							srcRange := trav.SourceRange()
							wrapped = fmt.Errorf("failed to deploy package %q at %s: %w",
								pkg.Name, srcRange.String(), err)
						}
					}
					levelErrs.Add(wrapped)
					// Returning the error makes errgroup cancel gctx so queued
					// siblings short-circuit; we drop g.Wait's return on the
					// floor in favour of the joined levelErrs.
					return wrapped
				}

				o.streams.Info("package deployed", "name", pkg.Name)
				return nil
			})
		}

		_ = g.Wait()
		if err := levelErrs.Err(); err != nil {
			return err
		}

		o.streams.Debug("deployment level complete", "level", levelIdx+1)
	}

	return nil
}
