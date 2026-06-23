// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// Test infrastructure
// ----------------------------------------------------------------------------
//
// The orchestrator is exercised through two purpose-built deploy callables:
//
//   - recordingDeploy: synchronous; records every call (pkg + ctx) and returns
//     a per-package error from the supplied map. Use for tests that don't need
//     to observe parallelism (DAG order, error wrapping, ctx propagation,
//     empty bundle, etc.).
//
//   - gatedDeploy: each call blocks on a per-package channel until the test
//     releases it. Use for tests that need to observe concurrency invariants
//     deterministically (max in-flight, scheduler-stop-on-failure, in-flight
//     completes-after-failure, level barriers).
//
// Together these replace the old ConcurrentMockDeployer, which did its own
// sleeping and ctx.Done() handling — making it hard to tell whether tests
// were verifying the orchestrator or the mock.

// deployCall records a single invocation of a recordingDeploy callable.
type deployCall struct {
	Pkg *Package
	Ctx context.Context
}

// deployFunc is the per-package deploy callable shape the orchestrator tests inject.
type deployFunc = func(ctx context.Context, pkg *Package, opts DeployPackageOptions) error

// fakeDeployer adapts a deployFunc into a Deployer so the orchestrator (which now
// calls Deployer.DeployPackage) can be driven by a mock in unit tests.
type fakeDeployer struct{ deploy deployFunc }

func (f fakeDeployer) DeployPackage(ctx context.Context, pkg *Package, opts DeployPackageOptions) error {
	return f.deploy(ctx, pkg, opts)
}

func (fakeDeployer) DeployBundle(context.Context, *UDSBundle, DeployOptions) (*DeployResult, error) {
	panic("fakeDeployer.DeployBundle is not used by orchestrator tests")
}

// recordingDeploy returns a synchronous deploy callable plus a snapshot getter.
// returns may map package names to errors that should be returned for that
// package; unset names return nil. The callable does no work — it just records
// and returns. Safe to invoke from multiple goroutines.
func recordingDeploy(returns map[string]error) (deployFunc, func() []deployCall) {
	var (
		mu    sync.Mutex
		calls []deployCall
	)
	fn := func(ctx context.Context, pkg *Package, _ DeployPackageOptions) error {
		mu.Lock()
		calls = append(calls, deployCall{Pkg: pkg, Ctx: ctx})
		mu.Unlock()
		return returns[pkg.Name]
	}
	snapshot := func() []deployCall {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(calls)
	}
	return fn, snapshot
}

// callNames extracts package names from a deployCall slice in call order.
func callNames(calls []deployCall) []string {
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.Pkg.Name
	}
	return out
}

// gatedDeploy is a deploy callable whose every invocation blocks on a per-
// package channel until the test releases it. This lets tests observe
// orchestrator scheduling deterministically — assertions become "exactly N
// packages have entered DeployPackage" rather than "no more than N entered
// during a 50ms sleep".
type gatedDeploy struct {
	mu          sync.Mutex
	inFlight    int
	maxInFlight int
	entered     []string

	entries chan string                   // signals each DeployPackage entry
	release map[string]chan releaseSignal // per-package release channel
}

type releaseSignal struct{ err error }

// newGatedDeploy builds a gatedDeploy that knows about the listed packages.
// All channels are buffered so tests can pre-release packages before they enter.
func newGatedDeploy(names ...string) *gatedDeploy {
	g := &gatedDeploy{
		entries: make(chan string, len(names)),
		release: make(map[string]chan releaseSignal, len(names)),
	}
	for _, name := range names {
		g.release[name] = make(chan releaseSignal, 1)
	}
	return g
}

func (g *gatedDeploy) deploy(ctx context.Context, pkg *Package, _ DeployPackageOptions) error {
	g.mu.Lock()
	g.inFlight++
	if g.inFlight > g.maxInFlight {
		g.maxInFlight = g.inFlight
	}
	g.entered = append(g.entered, pkg.Name)
	g.mu.Unlock()

	g.entries <- pkg.Name

	var err error
	select {
	case sig := <-g.release[pkg.Name]:
		err = sig.err
	case <-ctx.Done():
		err = ctx.Err()
	}

	g.mu.Lock()
	g.inFlight--
	g.mu.Unlock()
	return err
}

// waitForEntries blocks until n packages have called DeployPackage and returns
// their names in entry order. Fails the test if the deadline is reached first.
func (g *gatedDeploy) waitForEntries(t *testing.T, n int) []string {
	t.Helper()
	got := make([]string, 0, n)
	for i := 0; i < n; i++ {
		select {
		case name := <-g.entries:
			got = append(got, name)
		case <-time.After(time.Second):
			t.Fatalf("waited 1s for %d entries, got %d (%v)", n, i, got)
		}
	}
	return got
}

// assertNoMoreEntries fails if any additional DeployPackage call lands within
// the given window. Use to prove the orchestrator stopped scheduling.
func (g *gatedDeploy) assertNoMoreEntries(t *testing.T, within time.Duration) {
	t.Helper()
	select {
	case name := <-g.entries:
		t.Fatalf("unexpected entry: %s", name)
	case <-time.After(within):
	}
}

// releasePkg unblocks the named package; its DeployPackage call returns err.
func (g *gatedDeploy) releasePkg(name string, err error) {
	g.release[name] <- releaseSignal{err: err}
}

// releaseAll releases every package with nil error. Idempotent.
func (g *gatedDeploy) releaseAll() {
	for _, ch := range g.release {
		select {
		case ch <- releaseSignal{}:
		default: // already released
		}
	}
}

// MaxInFlight returns the peak number of concurrent DeployPackage calls observed.
func (g *gatedDeploy) MaxInFlight() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.maxInFlight
}

// EnteredNames returns every package that entered DeployPackage in order.
func (g *gatedDeploy) EnteredNames() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.entered)
}

// runDeployOrchestrator builds a deployOrchestrator wired to a deploy callable
// and runs it under t.Context(). Mirrors what ZarfDeployer.DeployBundle does
// in production minus the trivial DeployResult wrapping.
func runDeployOrchestrator(t *testing.T, b *UDSBundle, deploy deployFunc, concurrency int) error {
	t.Helper()
	return newOrchestratorForTest(t, b, deploy, concurrency).Run(t.Context())
}

// newOrchestratorForTest builds a deployOrchestrator from a bundle and deploy
// callable. Used by tests that need to drive Run with a custom context.
func newOrchestratorForTest(t *testing.T, b *UDSBundle, deploy deployFunc, concurrency int) *deployOrchestrator {
	t.Helper()
	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, b)
	require.NoError(t, err)
	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)
	return newDeployOrchestrator(fakeDeployer{deploy: deploy}, dag, levels, concurrency, DeployPackageOptions{
		Config: newDeployTestConfig(concurrency),
	}, iostreams.IOStreams{})
}

// newDeployTestConfig returns a minimal valid UDSBundleConfig for unit tests.
func newDeployTestConfig(concurrency int) *UDSBundleConfig {
	return &UDSBundleConfig{
		Global:  &GlobalOptions{LogLevel: "info"},
		Options: &ConfigOptions{Concurrency: concurrency, TmpDir: "/tmp"},
	}
}

// startOrchestrator runs the orchestrator on a goroutine and returns a channel
// that receives Run's error. Used by gated tests that need to observe progress
// while Run is still executing.
func startOrchestrator(t *testing.T, b *UDSBundle, deploy deployFunc, concurrency int) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	orch := newOrchestratorForTest(t, b, deploy, concurrency)
	go func() { done <- orch.Run(t.Context()) }()
	return done
}

// ----------------------------------------------------------------------------
// Synchronous tests (recordingDeploy)
// ----------------------------------------------------------------------------

func TestDeployOrchestrator_RespectsDAGOrder(t *testing.T) {
	t.Parallel()

	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "order-test"},
		Packages: []Package{
			{Name: "a", Source: "oci://example/a:v1"},
			{Name: "b", Source: "oci://example/b:v1", DependsOn: []PackageRef{{Name: "a"}}},
			{Name: "c", Source: "oci://example/c:v1", DependsOn: []PackageRef{{Name: "b"}}},
		},
	}

	deploy, snapshot := recordingDeploy(nil)
	require.NoError(t, runDeployOrchestrator(t, b, deploy, 10))
	assert.Equal(t, []string{"a", "b", "c"}, callNames(snapshot()),
		"linear chain must deploy in strict order")
}

func TestDeployOrchestrator_ErrorInLevelPreventsNextLevel(t *testing.T) {
	t.Parallel()

	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "level-stop-test"},
		Packages: []Package{
			{Name: "base", Source: "oci://example/base:v1"},
			{Name: "top", Source: "oci://example/top:v1", DependsOn: []PackageRef{{Name: "base"}}},
		},
	}

	deploy, snapshot := recordingDeploy(map[string]error{"base": errors.New("base failed")})

	err := runDeployOrchestrator(t, b, deploy, 10)
	require.ErrorContains(t, err, "base failed")

	calls := callNames(snapshot())
	assert.Contains(t, calls, "base")
	assert.NotContains(t, calls, "top",
		"level 1 packages must not start when level 0 fails")
}

func TestDeployOrchestrator_SinglePackage(t *testing.T) {
	t.Parallel()

	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "single"},
		Packages: []Package{{Name: "only", Source: "oci://example/only:v1"}},
	}

	deploy, snapshot := recordingDeploy(nil)
	require.NoError(t, runDeployOrchestrator(t, b, deploy, 10))
	assert.Equal(t, []string{"only"}, callNames(snapshot()))
}

func TestDeployOrchestrator_PreCancelledContext(t *testing.T) {
	t.Parallel()

	// Cancel the ctx before Run is called. The orchestrator must return
	// ctx.Err() at the level boundary without invoking the deploy callable.
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "pre-cancelled"},
		Packages: []Package{
			{Name: "a", Source: "oci://example/a:v1"},
			{Name: "b", Source: "oci://example/b:v1"},
		},
	}

	deploy, snapshot := recordingDeploy(nil)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := newOrchestratorForTest(t, b, deploy, 10).Run(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, snapshot(),
		"no packages should be invoked when context is pre-cancelled")
}

func TestDeployOrchestrator_ErrorWrapping(t *testing.T) {
	t.Parallel()

	// A failed deploy must surface as an error that:
	//   1) wraps the underlying error (errors.Is succeeds against the original)
	//   2) names the failing package in its message
	//   3) preserves the underlying error message
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "wrap-test"},
		Packages: []Package{{Name: "broken", Source: "oci://example/broken:v1"}},
	}

	rootErr := errors.New("zarf exploded")
	deploy, _ := recordingDeploy(map[string]error{"broken": rootErr})

	err := runDeployOrchestrator(t, b, deploy, 1)
	require.ErrorIs(t, err, rootErr, "error should wrap the underlying deploy error")
	assert.Contains(t, err.Error(), `"broken"`,
		"error should name the failing package")
	assert.Contains(t, err.Error(), "zarf exploded",
		"error should include the underlying message")
}

func TestDeployOrchestrator_EmptyBundle(t *testing.T) {
	t.Parallel()

	// Bundle with zero packages produces zero levels. Run should return nil
	// without invoking the deploy callable.
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "empty"},
		Packages: nil,
	}

	deploy, snapshot := recordingDeploy(nil)
	require.NoError(t, runDeployOrchestrator(t, b, deploy, 10))
	assert.Empty(t, snapshot(),
		"no packages should be invoked for an empty bundle")
}

func TestDeployOrchestrator_PassesParentContext(t *testing.T) {
	t.Parallel()

	// Run must propagate the parent ctx into the deploy callable so user
	// cancellation (Ctrl+C) reaches in-flight deploys. Verify by capturing
	// the ctx the orchestrator hands to deploy and comparing identities.
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "ctx-prop"},
		Packages: []Package{{Name: "only", Source: "oci://example/only:v1"}},
	}

	parent, cancel := context.WithCancel(t.Context())
	defer cancel()

	deploy, snapshot := recordingDeploy(nil)
	require.NoError(t, newOrchestratorForTest(t, b, deploy, 1).Run(parent))

	calls := snapshot()
	require.Len(t, calls, 1)
	assert.Same(t, parent, calls[0].Ctx,
		"orchestrator must hand the parent ctx (not gctx) to the deploy callable")
}

// ----------------------------------------------------------------------------
// Concurrency tests (gatedDeploy)
// ----------------------------------------------------------------------------

func TestDeployOrchestrator_ParallelWithinLevel(t *testing.T) {
	t.Parallel()

	// Diamond: base -> (left, right) -> top. Verify left and right run
	// concurrently in level 1 by observing both enter before either is
	// released.
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "parallel-test"},
		Packages: []Package{
			{Name: "base", Source: "oci://example/base:v1"},
			{Name: "left", Source: "oci://example/left:v1", DependsOn: []PackageRef{{Name: "base"}}},
			{Name: "right", Source: "oci://example/right:v1", DependsOn: []PackageRef{{Name: "base"}}},
			{Name: "top", Source: "oci://example/top:v1", DependsOn: []PackageRef{{Name: "left"}, {Name: "right"}}},
		},
	}

	g := newGatedDeploy("base", "left", "right", "top")
	done := startOrchestrator(t, b, g.deploy, 10)

	level0 := g.waitForEntries(t, 1)
	assert.Equal(t, []string{"base"}, level0)
	g.releasePkg("base", nil)

	level1 := g.waitForEntries(t, 2)
	assert.ElementsMatch(t, []string{"left", "right"}, level1)
	g.releasePkg("left", nil)
	g.releasePkg("right", nil)

	level2 := g.waitForEntries(t, 1)
	assert.Equal(t, []string{"top"}, level2)
	g.releasePkg("top", nil)

	require.NoError(t, <-done)
	assert.Equal(t, 2, g.MaxInFlight(),
		"left and right should be in-flight simultaneously")
}

func TestDeployOrchestrator_ConcurrencyLimitRespected(t *testing.T) {
	t.Parallel()

	// Five independent packages, concurrency=2. At most 2 may be in-flight
	// at any time.
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "concurrency-test"},
		Packages: []Package{
			{Name: "a", Source: "oci://example/a:v1"},
			{Name: "b", Source: "oci://example/b:v1"},
			{Name: "c", Source: "oci://example/c:v1"},
			{Name: "d", Source: "oci://example/d:v1"},
			{Name: "e", Source: "oci://example/e:v1"},
		},
	}

	g := newGatedDeploy("a", "b", "c", "d", "e")
	done := startOrchestrator(t, b, g.deploy, 2)

	// First batch of 2 enters, then no more until we release a slot.
	first := g.waitForEntries(t, 2)
	g.assertNoMoreEntries(t, 50*time.Millisecond)

	// Release first[0] — exactly one new package should enter.
	g.releasePkg(first[0], nil)
	g.waitForEntries(t, 1)
	g.assertNoMoreEntries(t, 50*time.Millisecond)

	// Drain the rest.
	g.releaseAll()
	require.NoError(t, <-done)

	assert.Len(t, g.EnteredNames(), 5)
	assert.Equal(t, 2, g.MaxInFlight(),
		"in-flight count should never exceed the configured concurrency")
}

func TestDeployOrchestrator_ConcurrencyOneSerializesLevel(t *testing.T) {
	t.Parallel()

	// Three independent packages, concurrency=1. Exactly one must be in-flight
	// at any moment.
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "serial-test"},
		Packages: []Package{
			{Name: "a", Source: "oci://example/a:v1"},
			{Name: "b", Source: "oci://example/b:v1"},
			{Name: "c", Source: "oci://example/c:v1"},
		},
	}

	g := newGatedDeploy("a", "b", "c")
	done := startOrchestrator(t, b, g.deploy, 1)

	for range 3 {
		entered := g.waitForEntries(t, 1)
		g.assertNoMoreEntries(t, 30*time.Millisecond)
		g.releasePkg(entered[0], nil)
	}

	require.NoError(t, <-done)
	assert.Equal(t, 1, g.MaxInFlight(),
		"concurrency=1 should never exceed 1 in-flight package")
}

func TestDeployOrchestrator_WideParallel(t *testing.T) {
	t.Parallel()

	// Eight independent packages, concurrency=4. Verify exactly 4 enter
	// before any release, then drain.
	pkgs := make([]Package, 8)
	names := make([]string, 8)
	for i := range pkgs {
		name := fmt.Sprintf("pkg%d", i)
		pkgs[i] = Package{Name: name, Source: fmt.Sprintf("oci://example/%s:v1", name)}
		names[i] = name
	}
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "wide-parallel"},
		Packages: pkgs,
	}

	g := newGatedDeploy(names...)
	done := startOrchestrator(t, b, g.deploy, 4)

	g.waitForEntries(t, 4)
	g.assertNoMoreEntries(t, 50*time.Millisecond)

	g.releaseAll()
	require.NoError(t, <-done)

	assert.Len(t, g.EnteredNames(), 8)
	assert.Equal(t, 4, g.MaxInFlight(),
		"in-flight count should reach the configured concurrency")
}

func TestDeployOrchestrator_FailureLetsInFlightFinish(t *testing.T) {
	t.Parallel()

	// Three independent packages, concurrency=10 starts them all. Package b
	// fails; a and c are mid-deploy and must complete normally — a sibling's
	// failure does not abort an already-running deploy.
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "error-test"},
		Packages: []Package{
			{Name: "a", Source: "oci://example/a:v1"},
			{Name: "b", Source: "oci://example/b:v1"},
			{Name: "c", Source: "oci://example/c:v1"},
		},
	}

	g := newGatedDeploy("a", "b", "c")
	done := startOrchestrator(t, b, g.deploy, 10)

	entered := g.waitForEntries(t, 3)
	assert.ElementsMatch(t, []string{"a", "b", "c"}, entered)

	// b fails, a and c return successfully.
	failErr := errors.New("deploy failed")
	g.releasePkg("b", failErr)
	g.releasePkg("a", nil)
	g.releasePkg("c", nil)

	err := <-done
	require.ErrorIs(t, err, failErr,
		"the original deploy error should surface")
	assert.ElementsMatch(t, []string{"a", "b", "c"}, g.EnteredNames(),
		"all three packages must have entered DeployPackage")
}

func TestDeployOrchestrator_AggregatesMultipleFailuresInLevel(t *testing.T) {
	t.Parallel()

	// Three independent packages, concurrency=10 starts them all. Two fail
	// concurrently; the orchestrator must surface BOTH errors via errors.Join,
	// not just the first one to reach Wait. errgroup's Wait alone returns only
	// the first non-nil error — that would silently drop sibling failures.
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "multi-fail-test"},
		Packages: []Package{
			{Name: "a", Source: "oci://example/a:v1"},
			{Name: "b", Source: "oci://example/b:v1"},
			{Name: "c", Source: "oci://example/c:v1"},
		},
	}

	g := newGatedDeploy("a", "b", "c")
	done := startOrchestrator(t, b, g.deploy, 10)

	g.waitForEntries(t, 3)

	errA := errors.New("a exploded")
	errC := errors.New("c exploded")
	g.releasePkg("a", errA)
	g.releasePkg("b", nil)
	g.releasePkg("c", errC)

	err := <-done
	require.Error(t, err)
	require.ErrorIs(t, err, errA, "joined error should expose package a's failure")
	require.ErrorIs(t, err, errC, "joined error should expose package c's failure")
	assert.Contains(t, err.Error(), `"a"`)
	assert.Contains(t, err.Error(), `"c"`)
}

func TestDeployOrchestrator_FailureStopsScheduling(t *testing.T) {
	t.Parallel()

	// Six packages, concurrency=2. pkg0 fails; pkg1 succeeds; pkg2..pkg5
	// must NOT enter DeployPackage (the orchestrator stops scheduling once
	// gctx cancels and queued goroutines short-circuit on the gctx.Err()
	// re-check after slot acquisition).
	pkgs := make([]Package, 6)
	names := make([]string, 6)
	for i := range pkgs {
		name := fmt.Sprintf("pkg%d", i)
		pkgs[i] = Package{Name: name, Source: fmt.Sprintf("oci://example/%s:v1", name)}
		names[i] = name
	}
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "stop-scheduling"},
		Packages: pkgs,
	}

	g := newGatedDeploy(names...)
	done := startOrchestrator(t, b, g.deploy, 2)

	first := g.waitForEntries(t, 2)
	assert.ElementsMatch(t, []string{"pkg0", "pkg1"}, first)

	// Release pkg0 with an error and assert no further entries BEFORE
	// releasing pkg1. The order matters: if pkg1's slot were freed first
	// (because we released it concurrently), pkg2 could acquire that slot
	// and run its gctx.Err() check before pkg0's failure has propagated —
	// missing the cancel and entering DeployPackage. By keeping pkg1 in-
	// flight while we assert, the only slot pkg2 can possibly acquire is
	// the one pkg0 freed AFTER errgroup observed the failure and cancelled
	// gctx, so pkg2's gctx.Err() check fires deterministically.
	g.releasePkg("pkg0", errors.New("pkg0 failed"))
	g.assertNoMoreEntries(t, 50*time.Millisecond)

	// Drain pkg1 so g.Wait can return.
	g.releasePkg("pkg1", nil)

	err := <-done
	require.ErrorContains(t, err, "pkg0 failed")
	assert.ElementsMatch(t, []string{"pkg0", "pkg1"}, g.EnteredNames(),
		"only the first concurrency-bounded batch should be invoked")
}

func TestDeployOrchestrator_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Cancel the parent ctx mid-deploy. The gated deploy selects on ctx.Done,
	// so cancellation should propagate and surface as context.Canceled.
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "cancel-test"},
		Packages: []Package{{Name: "slow", Source: "oci://example/slow:v1"}},
	}

	g := newGatedDeploy("slow")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	orch := newOrchestratorForTest(t, b, g.deploy, 1)
	done := make(chan error, 1)
	go func() { done <- orch.Run(ctx) }()

	g.waitForEntries(t, 1)
	cancel()

	err := <-done
	require.ErrorIs(t, err, context.Canceled)
}

func TestDeployOrchestrator_ContextDeadlineExceeded(t *testing.T) {
	t.Parallel()

	// Deadline-based cancellation should surface as context.DeadlineExceeded.
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "deadline-test"},
		Packages: []Package{{Name: "slow", Source: "oci://example/slow:v1"}},
	}

	g := newGatedDeploy("slow")
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	orch := newOrchestratorForTest(t, b, g.deploy, 1)
	done := make(chan error, 1)
	go func() { done <- orch.Run(ctx) }()

	g.waitForEntries(t, 1)
	// Don't release — let the deadline fire.

	err := <-done
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestDeployOrchestrator_ConcurrentOutputIsClean(t *testing.T) {
	t.Parallel()

	// Verify parallel deploys within a level write to the shared IOStreams without
	// corrupting each other. The race detector catches unsynchronized concurrent
	// access to the underlying bytes.Buffer; distinct per-package tokens detect
	// byte-level interleaving even when -race is not active.
	const pkgCount = 10
	const writesPerPkg = 50
	const tokenLen = 5 // "[p00]" through "[p09]"

	var buf bytes.Buffer
	streams := iostreams.New(nil, nil, &buf)

	pkgs := make([]Package, pkgCount)
	for i := range pkgs {
		pkgs[i] = Package{Name: fmt.Sprintf("p%02d", i), Source: "oci://example.com/p:v1"}
	}
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "output-test"},
		Packages: pkgs,
	}

	deploy := func(_ context.Context, pkg *Package, opts DeployPackageOptions) error {
		token := fmt.Sprintf("[%s]", pkg.Name)
		for i := 0; i < writesPerPkg; i++ {
			_, _ = fmt.Fprint(opts.Streams.ErrOut(), token)
		}
		return nil
	}

	dag, err := BuildDependencyGraph(t.Context(), iostreams.IOStreams{}, b)
	require.NoError(t, err)
	levels, err := dag.TopologicalLevels()
	require.NoError(t, err)

	pkgOpts := DeployPackageOptions{
		Config:  newDeployTestConfig(pkgCount),
		Streams: streams,
	}
	orch := newDeployOrchestrator(fakeDeployer{deploy: deploy}, dag, levels, pkgCount, pkgOpts, streams)
	require.NoError(t, orch.Run(t.Context()))

	got := buf.String()
	assert.Len(t, got, pkgCount*writesPerPkg*tokenLen, "byte count must be exact")
	for _, pkg := range pkgs {
		token := fmt.Sprintf("[%s]", pkg.Name)
		assert.Equal(t, writesPerPkg, strings.Count(got, token),
			"token %s must appear exactly %d times without splitting", token, writesPerPkg)
	}
}
