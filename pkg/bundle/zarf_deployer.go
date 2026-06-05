// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
	"github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/feature"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/value"
)

// zarfGlobalsOnce guards the one-time, process-wide Zarf configuration applied in NewZarfDeployer.
var zarfGlobalsOnce sync.Once

var _ Deployer = (*ZarfDeployer)(nil)

// Output synchronization
//
// Parallel deploys within a level have N goroutines all writing human-readable
// output destined for a single terminal, so synchronization is required
// somewhere in the pipeline to keep writes from corrupting one another. The
// choice of granularity is a UX trade-off:
//
//   - byte-level (this implementation, via syncWriter): every Write call is
//     serialized. Cheapest and simplest; preserves real-time output but
//     individual lines from different packages can still interleave on screen.
//   - line-level: lines stay atomic but lines from different packages still
//     intermix.
//   - package-level: each package writes to its own buffer, flushed under a
//     mutex when the package completes. Coherent per-package log blocks at
//     the cost of no live progress within a package.
//
// We picked byte-level (syncWriter) because Zarf's logger emits frequent
// progress updates that users expect to see live during a deploy; deferring
// per-package output until completion would feel like the deploy stalled.
// Mid-line interleaving is rare in practice (Zarf's writer emits one line
// per Write) and acceptable given the live-feedback gain.

// syncWriter wraps an io.Writer with a mutex so concurrent goroutines
// (parallel package deploys within a level) do not corrupt each other's
// writes. See the "Output synchronization" doc above for the rationale.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (sw *syncWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

// ZarfDeployer implements Deployer using the Zarf Go library.
// Reference: .ai/example-repos/uds-cli/src/pkg/bundle/deploy.go lines 38-165
type ZarfDeployer struct {
	// streams carries the diagnostic sink (streams.ErrOut) handed to the Zarf
	// logger and the leveled logger used for UDS-side diagnostics. streams.ErrOut
	// is wrapped in a syncWriter by NewZarfDeployer so concurrent DeployPackage
	// calls produce clean output.
	streams iostreams.IOStreams

	// Loader, when non-nil, is used instead of NewPackageSource to obtain each
	// package's layout. Used when deploying from a pre-extracted workspace (ADR-0009).
	Loader PackageLayoutLoader
}

// NewZarfDeployer creates a ZarfDeployer.
// When loader is nil, packages are loaded from their declared source using
// SourcePackageLayoutLoader. For local artifact deploys, provide a
// PackageLayoutLoader implementation that loads packages from the extracted
// artifact's OCI layout instead of pulling from the declared source.
func NewZarfDeployer(streams iostreams.IOStreams, loader PackageLayoutLoader) *ZarfDeployer {
	// Guard before wrapping in syncWriter: a nil ErrOut would otherwise become a
	// non-nil *syncWriter over a nil writer and panic on first write.
	if streams.ErrOut == nil {
		streams.ErrOut = io.Discard
	}
	streams.ErrOut = &syncWriter{w: streams.ErrOut}
	zarfGlobalsOnce.Do(func() {
		// Route Zarf action subprocess output through the context logger instead of
		// the process's raw os.Stdout/os.Stderr.
		config.CommonOptions.PreferLogger = true
		// Enable the Zarf "values" feature flag so packager.Deploy accepts
		// DeployOptions.Values (Helm values from values_files).
		_ = feature.Set([]feature.Feature{{
			Name:    feature.Values,
			Enabled: true,
			Stage:   feature.Alpha,
		}})
	})
	d := &ZarfDeployer{
		Loader: loader,
	}
	d.streams = streams
	return d
}

// DeployBundle deploys the bundle's packages in topological order, parallelising
// within levels and serialising across them.
func (d *ZarfDeployer) DeployBundle(ctx context.Context, b *UDSBundle, opts DeployOptions) (*DeployResult, error) {
	if err := ValidateConfig(opts.Config); err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errNil("bundle")
	}
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("bundle validation failed: %w", err)
	}
	s := logger.Bind(d.streams, opts.Config.Global.LogLevel)

	dag, err := BuildDependencyGraph(ctx, s, b)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	levels, err := dag.TopologicalLevels()
	if err != nil {
		return nil, fmt.Errorf("failed to compute deployment levels: %w", err)
	}
	s.Debug("dependency graph built", "levels", len(levels))

	concurrency := opts.Config.Options.Concurrency

	pkgOpts := DeployPackageOptions{
		Config:    opts.Config,
		BundleDir: filepath.Dir(opts.BundlePath),
		Prompt:    opts.Prompt,
		Streams:   s,
	}

	s.Info("deploying bundle", "packages", len(b.Packages), "levels", len(levels), "concurrency", concurrency)

	orch := newDeployOrchestrator(d.DeployPackage, dag, levels, concurrency, pkgOpts, s)
	if err := orch.Run(ctx); err != nil {
		return nil, err
	}

	return &DeployResult{
		BundleName: b.Metadata.Name,
		Packages:   len(b.Packages),
	}, nil
}

// DeployPackage deploys a single Zarf package using the Zarf Go library.
func (d *ZarfDeployer) DeployPackage(ctx context.Context, pkg *Package, opts DeployPackageOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if pkg == nil {
		return errNil("package")
	}
	log := logger.Bind(d.streams, opts.Config.Global.LogLevel)
	log.Info("deploying zarf package", "name", pkg.Name, "source", pkg.Source)

	ctx = newZarfLoggerContext(ctx, log.ErrOut, opts.Config.Global.LogLevel)

	pkgTmp, err := os.MkdirTemp(opts.Config.Options.TmpDir, "zarf-pkg-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(pkgTmp); err != nil {
			log.Warn("failed to remove temporary directory", "path", pkgTmp, "error", err)
		}
	}()

	zarfValues, setVars, err := d.prepareValuesAndVariables(ctx, log, pkg, opts)
	if err != nil {
		return err
	}

	loader := d.Loader
	if loader == nil {
		loader = &SourcePackageLayoutLoader{configOpts: *opts.Config.Options, bundleDir: opts.BundleDir}
	}

	pkgLayout, err := loader.LoadPackageLayout(ctx, log, pkg, pkgTmp)
	if err != nil {
		return err
	}
	defer func() {
		if err := pkgLayout.Cleanup(); err != nil {
			log.Warn("failed to clean up package layout", "name", pkg.Name, "error", err)
		}
	}()

	// IsInteractive matches Prompt - only show interactive prompts when user opted in via --prompt
	deployOpts := packager.DeployOptions{
		Values:            zarfValues, // Helm chart values from values_files
		SetVariables:      setVars,    // Zarf ###ZARF_PKG_VAR_*### passthrough
		IsInteractive:     opts.Prompt,
		NamespaceOverride: pkg.Namespace, // empty string is fine - Zarf ignores it
	}

	log.Info("deploying zarf package to cluster", "name", pkg.Name)

	_, err = packager.Deploy(ctx, pkgLayout, deployOpts)
	if err != nil {
		return fmt.Errorf("failed to deploy package %q: %w", pkg.Name, err)
	}

	log.Info("package deployed", "name", pkg.Name)
	return nil
}

// prepareValuesAndVariables resolves, templates, and parses values_files for a package,
// and flattens config variables for Zarf ###ZARF_PKG_VAR_*### substitution.
// Temporary files created during templating are cleaned up before this method returns,
// since value.ParseFiles reads them into memory before returning.
func (d *ZarfDeployer) prepareValuesAndVariables(ctx context.Context, streams iostreams.IOStreams, pkg *Package, opts DeployPackageOptions) (zarfValues value.Values, setVars map[string]string, err error) {
	var configVars Variables
	if opts.Config != nil {
		configVars = opts.Config.Variables
	}

	var loadedFileCount int
	if len(pkg.ValuesFiles) > 0 {
		// 1. Resolve relative paths against the bundle directory
		resolved := resolveValuesFiles(pkg.ValuesFiles, opts.BundleDir)

		// 2. Template {{ .vars.* }} placeholders using config variables
		var filesToParse []string
		filesToParse, err = templateValuesFiles(ctx, resolved, configVars, opts.Config.Options.TmpDir)
		// Temp files are fully consumed by ParseFiles below or any subsequent error; clean up on return.
		if configVars != nil {
			defer cleanupTempFiles(ctx, streams, filesToParse)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to template values files for package %q: %w", pkg.Name, err)
		}

		// 3. Parse YAML files with Zarf's value package
		zarfValues, err = value.ParseFiles(ctx, filesToParse, value.ParseFilesOptions{})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse values files for package %q: %w", pkg.Name, err)
		}
		loadedFileCount = len(filesToParse)
	}

	// Flatten top-level scalar variables for Zarf ###ZARF_PKG_VAR_*### substitution.
	// Non-scalars are skipped here and flow through values_files instead.
	setVars, err = configVars.Flatten()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to flatten variables for package %q: %w", pkg.Name, err)
	}

	if loadedFileCount > 0 {
		streams.Debug("loaded values files", "package", pkg.Name, "count", loadedFileCount)
	}
	return zarfValues, setVars, nil
}

// resolveValuesFiles resolves values file paths relative to bundleDir.
// Absolute paths are returned unchanged. nil input returns nil.
func resolveValuesFiles(files []string, bundleDir string) []string {
	if files == nil {
		return nil
	}
	resolved := make([]string, len(files))
	for i, f := range files {
		if filepath.IsAbs(f) {
			resolved[i] = filepath.Clean(f)
		} else {
			resolved[i] = filepath.Join(bundleDir, f)
		}
	}
	return resolved
}

// templateValuesFiles renders {{ .vars.* }} templates in values files using vars
// from config.uds.hcl. Returns paths to temporary files with rendered content.
// The caller is responsible for calling cleanupTempFiles on the returned paths.
//
// Template context: { "vars": vars }
// Access: {{ .vars.domain }}, {{ .vars.logging.vectorEnabled }}, etc.
//
// Templates are rendered by Go's stdlib text/template — authors get range, if,
// with, index, printf, dot-access, pipe, and whitespace-trim markers.
// Lists and maps are rendered with explicit range loops.
//
// Missing keys produce a clear error (template.Option("missingkey=error")).
// If vars is nil, the original file paths are returned unchanged (no temp copies created).
func templateValuesFiles(_ context.Context, files []string, vars Variables, tmpDir string) ([]string, error) {
	if vars == nil {
		return files, nil
	}

	// Variables is map[string]any underneath, and Go templates traverse named map
	// types via reflection at any depth, so no conversion of nested levels is needed.
	data := map[string]any{"vars": vars}
	result := make([]string, 0, len(files))

	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return result, fmt.Errorf("failed to read values file %q: %w", f, err)
		}

		tmpl, err := template.New(filepath.Base(f)).Option("missingkey=error").Parse(string(src))
		if err != nil {
			return result, fmt.Errorf("failed to parse template in values file %q: %w", f, err)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return result, fmt.Errorf("failed to render values file %q: %w", f, err)
		}

		tmp, err := os.CreateTemp(tmpDir, "uds-values-*.yaml")
		if err != nil {
			return result, fmt.Errorf("failed to create temp file for values: %w", err)
		}
		// Append immediately so the caller's cleanup catches the file even if Write/Close fails.
		result = append(result, tmp.Name())
		if _, err := tmp.Write(buf.Bytes()); err != nil {
			_ = tmp.Close()
			return result, fmt.Errorf("failed to write temp values file: %w", err)
		}
		if err := tmp.Close(); err != nil {
			return result, fmt.Errorf("failed to close temp values file: %w", err)
		}
	}
	return result, nil
}

// cleanupTempFiles removes temporary files created by templateValuesFiles.
// Removal errors are logged but not returned, consistent with existing cleanup patterns.
func cleanupTempFiles(_ context.Context, streams iostreams.IOStreams, files []string) {
	for _, f := range files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			streams.Warn("failed to remove temp file", "path", f, "error", err)
		}
	}
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
