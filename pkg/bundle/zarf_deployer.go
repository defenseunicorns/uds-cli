// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/defenseunicorns/pkg/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/logger"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
)

// ZarfDeployer implements Deployer using the Zarf Go library.
// Reference: .ai/example-repos/uds-cli/src/pkg/bundle/deploy.go lines 38-165
type ZarfDeployer struct {
	// TempDir is the base directory for temporary files
	TempDir string

	// Out is the writer for output messages
	Out io.Writer
}

// NewZarfDeployer creates a new ZarfDeployer.
func NewZarfDeployer(tempDir string, out io.Writer) *ZarfDeployer {
	return &ZarfDeployer{
		TempDir: tempDir,
		Out:     out,
	}
}

// DeployPackage deploys a single Zarf package using the Zarf Go library.
// In this milestone, only OCI-sourced packages are supported (no values files or templating).
//
// Reference implementation: .ai/example-repos/uds-cli/src/pkg/bundle/deploy.go
func (d *ZarfDeployer) DeployPackage(ctx context.Context, pkg *Package, opts DeployPackageOptions) error {
	// Validate that the source is an OCI reference
	if !IsOCIReference(pkg.Source) {
		return fmt.Errorf("package %q has unsupported source type: %s (only oci:// sources are supported)", pkg.Name, pkg.Source)
	}

	// Set up Zarf logger in context
	ctx = d.setupLoggerContext(ctx)

	// Create temp directory for this package
	pkgTmp, err := os.MkdirTemp(d.TempDir, "zarf-pkg-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		err := os.RemoveAll(pkgTmp)
		if err != nil {
			_, _ = fmt.Fprintf(d.Out, "  Warning: failed to remove the temporary directory: %v\n", err)
		}
	}()

	// Load the package from its OCI source
	_, _ = fmt.Fprintf(d.Out, "  Pulling package from %s\n", pkg.Source)

	pkgLayout, err := d.pullAndLoadPackage(ctx, pkg.Source, pkgTmp, pkg.OptionalComponents)
	if err != nil {
		return fmt.Errorf("failed to load package %q from %s: %w", pkg.Name, pkg.Source, err)
	}
	defer func() {
		err := pkgLayout.Cleanup()
		if err != nil {
			_, _ = fmt.Fprintf(d.Out, "  Warning: failed to clean up package layout: %v\n", err)
		}
	}()

	// Build deploy options for Zarf
	// IsInteractive is the opposite of Confirm - if user confirmed, we don't need interactive prompts
	deployOpts := packager.DeployOptions{
		IsInteractive:     !opts.Confirm,
		NamespaceOverride: pkg.Namespace, // empty string is fine - Zarf ignores it
	}

	_, _ = fmt.Fprintf(d.Out, "  Deploying package %s...\n", pkg.Name)

	// Deploy using Zarf packager
	_, err = packager.Deploy(ctx, pkgLayout, deployOpts)
	if err != nil {
		return fmt.Errorf("failed to deploy package %q: %w", pkg.Name, err)
	}

	return nil
}

// setupLoggerContext creates a context with a Zarf logger configured
func (d *ZarfDeployer) setupLoggerContext(ctx context.Context) context.Context {
	// Create a Zarf logger that writes to our output
	cfg := logger.Config{
		Level:       logger.Info,
		Format:      logger.FormatConsole,
		Destination: logger.Destination(d.Out),
		Color:       true,
	}
	l, err := logger.New(cfg)
	if err != nil {
		// Fall back to default logger if config fails
		l = logger.Default()
	}
	return logger.WithContext(ctx, l)
}

// pullAndLoadPackage pulls a Zarf package from an OCI registry and loads it into a PackageLayout.
// Reference: .ai/example-repos/uds-cli/src/pkg/sources/remote.go and zarf/src/pkg/zoci/
func (d *ZarfDeployer) pullAndLoadPackage(ctx context.Context, source, tmpDir string, optionalComponents []string) (*layout.PackageLayout, error) {
	// Strip oci:// prefix if present
	ociRef := TrimScheme(source)

	// Create platform for current architecture
	platform := ocispec.Platform{
		// TODO: This is a temporary approach that will be clarified after https://github.com/defenseunicorns/uds-cli/issues/23
		// is done.
		Architecture: runtime.GOARCH,
		OS:           oci.MultiOS,
	}

	_, _ = fmt.Fprintf(d.Out, "  Creating OCI remote for %s\n", ociRef)

	// Create remote client for the OCI registry
	remote, err := zoci.NewRemote(ctx, ociRef, platform)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCI remote: %w", err)
	}

	// Build component filter for optional components.
	// Always use ForDeploy so that optional components are excluded by default
	// and only included when explicitly requested via optional_components.
	filter := BuildComponentFilter(optionalComponents)

	_, _ = fmt.Fprintf(d.Out, "  Assembling layers...\n")

	// Assemble layers to pull (all layers for now)
	layers, err := remote.AssembleLayers(ctx, nil, false, zoci.AllLayers)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble layers: %w", err)
	}

	_, _ = fmt.Fprintf(d.Out, "  Pulling %d layers to %s...\n", len(layers), tmpDir)

	// Pull the package to the temp directory
	_, err = remote.PullPackage(ctx, tmpDir, zoci.DefaultConcurrency, layers...)
	if err != nil {
		return nil, fmt.Errorf("failed to pull package: %w", err)
	}

	_, _ = fmt.Fprintf(d.Out, "  Loading package layout...\n")

	// Load the package layout from the pulled files
	layoutOpts := layout.PackageLayoutOptions{
		Filter: filter,
	}

	pkgLayout, err := layout.LoadFromDir(ctx, tmpDir, layoutOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to load package layout: %w", err)
	}

	return pkgLayout, nil
}

// BuildComponentFilter creates a component filter strategy from optional component names.
// When optionalComponents is empty, only Required and Default Zarf components are included.
// When optionalComponents lists component names, those are explicitly included alongside
// Required components. Use the "-name" prefix to explicitly exclude a component.
func BuildComponentFilter(optionalComponents []string) filters.ComponentFilterStrategy {
	return filters.Combine(
		filters.ForDeploy(strings.Join(optionalComponents, ","), false),
	)
}
