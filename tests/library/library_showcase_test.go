// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

// Package bundle_test provides library integration tests for the deploy hooks API.
//
// This file: TestLibraryDeploy_Showcase — canonical end-to-end usage example.
//
// === What this test is ===
//
// The single most representative demonstration of how a consumer (Remote Agent, Fleet, etc.)
// uses the CLI Next deploy library with custom hooks and a custom log sink. It reads
// top-to-bottom as copy-paste documentation and serves as the gold-standard reference for
// "correct usage". Hook functions are written as named top-level helpers (not inline
// closures) so the call site reads like prose.
//
// === Why we created it ===
//
// CLI-187 adds PackageDeployHooks and BundleDeployHooks to the library API — extension points
// that let consumers customize each package's layout and deploy options before cluster
// operations. The library also logs through an injected iostreams.IOStreams, so a consumer
// fully controls where diagnostics go. This test exercises the public API surface a consumer
// touches:
//   - NewZarfDeployer with a consumer-supplied IOStreams (log interception, see step 1)
//   - A PackageLayoutLoader (here a simple in-memory fake — no registry, no symlinks)
//   - Both PackageDeployHooks and BundleDeployHooks on DeployOptions
//   - DeployBundle(...) returning (*DeployResult, error)
//
// === Log interception ===
//
// The library never writes to a global stream; it logs through IOStreams.ErrOut. A consumer
// can point ErrOut at any io.Writer. Here we wrap an io.MultiWriter so the library's logs are
// BOTH captured in a buffer (to assert on / ship to a log aggregator) AND forwarded to stdout.
//
// === Note on the cluster seam ===
//
// The only test-only piece is the cluster stand-in wired via DeployOptions.PackageDeployFn,
// which runs the real per-package deploy (loader + hooks + layout mutation) with a fake
// cluster push so this runs in CI without k3d. Production consumers drop that field and the
// real DeployPackage (and packager.Deploy) runs against a cluster.
package bundle_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"

	bundle "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// ── Consumer-side deploy hooks, written as a consumer would write them. ──

// skipImagesAlreadyInZarfRegistry zeroes a package's images just before deploy: a consumer
// like the Remote Agent has already cross-mounted them into the Zarf registry, so
// packager.Deploy must not try to push them. This is the canonical PreDeploy mutation.
func skipImagesAlreadyInZarfRegistry(
	_ context.Context, _ *bundle.Package, pkgLayout *layout.PackageLayout,
	_ *packager.DeployOptions, _ *bundle.DeployPackageOptions,
) error {
	for i := range pkgLayout.Pkg.Components {
		pkgLayout.Pkg.Components[i].Images = []string{}
		pkgLayout.Pkg.Components[i].ImageArchives = []v1alpha1.ImageArchive{}
	}
	return nil
}

// consumerState is the consumer's own deployment state — the library never stores it.
type consumerState struct {
	mu               sync.Mutex
	startedBundle    string
	deployedPackages []string
	bundleComplete   bool
}

// onBundleDeployStart (bundle PreDeploy) runs once before anything: the consumer stamps the deploy.
func (s *consumerState) onBundleDeployStart(_ context.Context, b *bundle.UDSBundle, _ *bundle.DeployOptions) error {
	s.startedBundle = b.Metadata.Name
	return nil
}

// onPackageDeployed (package PostDeploy) runs once per package, possibly concurrently — so it locks.
func (s *consumerState) onPackageDeployed(_ context.Context, pkg *bundle.Package) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deployedPackages = append(s.deployedPackages, pkg.Name)
	return nil
}

// onBundleComplete (bundle PostDeploy) runs once after every package — single-threaded, no lock needed.
func (s *consumerState) onBundleComplete(_ context.Context, _ *bundle.UDSBundle) error {
	s.bundleComplete = true
	return nil
}

// bundlePackageNames returns the names of all packages in b, in order.
func bundlePackageNames(b *bundle.UDSBundle) []string {
	names := make([]string, len(b.Packages))
	for i, pkg := range b.Packages {
		names[i] = pkg.Name
	}
	return names
}

// deployCapture captures layouts passed to the ClusterDeployFn seam.
type deployCapture struct {
	mu      sync.Mutex
	layouts []*layout.PackageLayout
}

// capturingClusterDeploy returns a cluster-deploy stand-in that records each PackageLayout,
// replacing the real packager.Deploy so no cluster or registry is required.
func capturingClusterDeploy(c *deployCapture) func(context.Context, *layout.PackageLayout, packager.DeployOptions) error {
	return func(_ context.Context, l *layout.PackageLayout, _ packager.DeployOptions) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.layouts = append(c.layouts, l)
		return nil
	}
}

// TestLibraryDeploy_Showcase is the canonical end-to-end usage example. It reads
// top-to-bottom as "how to use the deploy library" from a consumer's perspective, and
// doubles as copy-paste documentation for Fleet/RA.
func TestLibraryDeploy_Showcase(t *testing.T) {
	// 1. Intercept the library's logs. The library logs through IOStreams.ErrOut and never
	//    to a global stream, so a consumer points ErrOut at any io.Writer. Here we wrap an
	//    io.MultiWriter so the logs are both captured (to inspect / ship elsewhere) and
	//    forwarded to stdout.
	var capturedLogs bytes.Buffer
	streams := iostreams.IOStreams{ErrOut: io.MultiWriter(os.Stdout, &capturedLogs)}

	// 2. Build the deployer with the log sink and a simple in-memory fake loader that returns
	//    an image-bearing package layout (no registry, no symlinks). imageLayoutLib and
	//    mkStaticLoader are shared test helpers; a real consumer supplies its own loader.
	deployer := bundle.NewZarfDeployer(streams, mkStaticLoader(imageLayoutLib(), nil))

	// 3. The bundle definition. In production this comes from
	//    NewHCLParser().ParseBundleFile(...); here we build it in-memory to keep the showcase
	//    self-contained.
	b := singlePkgBundle()

	// 4. Deploy with both hook pairs. PreDeploy zeroes the package images; PostDeploy hooks
	//    track progress. Config sets the log level the bound logger honours.
	cfg := newTestConfig()
	cfg.Global.LogLevel = "info"

	var deployed deployCapture
	consumer := &consumerState{}
	result, err := deployer.DeployBundle(t.Context(), b, bundle.DeployOptions{
		Config: cfg,
		// test-only: deploy each package for real (loader + hooks) but stand in for the
		// cluster push so this runs in CI without k3d. Production consumers DROP this field.
		PackageDeployFn: deployWithClusterStub(deployer, capturingClusterDeploy(&deployed)),
		BundleDeployHooks: bundle.BundleDeployHooks{
			PreDeploy:  consumer.onBundleDeployStart,
			PostDeploy: consumer.onBundleComplete,
		},
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy:  skipImagesAlreadyInZarfRegistry,
			PostDeploy: consumer.onPackageDeployed,
		},
	})
	require.NoError(t, err)

	// 5a. Hook outcomes: every package deployed, bundle hooks fired once each.
	assert.Equal(t, b.Metadata.Name, consumer.startedBundle, "Bundle.PreDeploy must record bundle name")
	assert.Equal(t, len(b.Packages), result.Packages, "all packages must be reported in result")
	assert.True(t, consumer.bundleComplete, "Bundle.PostDeploy must run once at the end")
	assert.ElementsMatch(t, bundlePackageNames(b), consumer.deployedPackages, "PostDeploy must fire once per package")

	// 5b. The PreDeploy mutation (zeroed images) reached the deploy call.
	deployed.mu.Lock()
	layouts := deployed.layouts
	deployed.mu.Unlock()
	require.NotEmpty(t, layouts, "the cluster seam must have received at least one layout")
	for _, l := range layouts {
		for _, c := range l.Pkg.Components {
			assert.Empty(t, c.Images, "component %q: Images must be zeroed by skipImagesAlreadyInZarfRegistry", c.Name)
			assert.Empty(t, c.ImageArchives, "component %q: ImageArchives must be zeroed", c.Name)
		}
	}

	// 5c. The consumer captured the library's own logs (and forwarded them to stdout above).
	logs := capturedLogs.String()
	assert.Contains(t, logs, "deploying bundle", "consumer must be able to capture the library's deploy logs")
	assert.True(t, strings.Contains(logs, "package deployed") || strings.Contains(logs, "deploying package"),
		"per-package deploy progress must reach the consumer's log sink")
}
