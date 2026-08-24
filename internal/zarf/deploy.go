// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"text/template"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/feature"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/value"
)

// PackageDeployHooks provides deployment extension points per package.
type PackageDeployHooks struct {
	// PreDeploy runs after layout loading and before cluster deployment. A
	// returned error skips deployment and PostDeploy.
	PreDeploy func(context.Context, *spec.Package, *layout.PackageLayout, *packager.DeployOptions, *DeployPackageOptions) error
	// PostDeploy runs after a successful package deployment.
	PostDeploy func(context.Context, *spec.Package) error
}

// BundleDeployHooks provides deployment extension points for a bundle.
type BundleDeployHooks struct {
	// PreDeploy runs before package deployment begins.
	PreDeploy func(context.Context, *spec.UDSBundle, *DeployOptions) error
	// PostDeploy runs after every selected package deploys successfully.
	PostDeploy func(context.Context, *spec.UDSBundle) error
}

// DeployOptions contains private options for deploying a bundle.
type DeployOptions struct {
	// Config is the resolved deployment configuration.
	Config *UDSBundleConfig
	// BundlePath identifies the bundle definition.
	BundlePath string
	// BundleDir resolves package-relative paths.
	BundleDir string
	// Packages restricts deployment when non-empty.
	Packages           []string
	BundleDeployHooks  BundleDeployHooks
	PackageDeployHooks PackageDeployHooks
	// PackageDeployFn replaces the complete per-package deployment path when non-nil.
	PackageDeployFn func(context.Context, *spec.Package, DeployPackageOptions) error
}

// DeployResult represents the result of deploying a bundle.
type DeployResult struct {
	BundleName string
	Packages   []string
}

// Deployer deploys individual packages or complete bundles.
type Deployer interface {
	// DeployPackage deploys one package after its dependencies are available.
	DeployPackage(context.Context, *spec.Package, DeployPackageOptions) error
	// DeployBundle deploys packages in dependency order with bounded parallelism.
	DeployBundle(context.Context, *spec.UDSBundle, DeployOptions) (*DeployResult, error)
}

// DeployPackageOptions contains options for deploying a single package.
type DeployPackageOptions struct {
	// Config is the merged configuration and is always non-nil.
	Config *UDSBundleConfig
	// BundleDir resolves package-relative paths.
	BundleDir string
	// PackageDeployHooks supplies optional package callbacks.
	PackageDeployHooks PackageDeployHooks
	// IsPartial reports whether the loaded layout omits checksum-referenced layers.
	IsPartial bool
	// ClusterDeployFn performs the cluster-side deployment; nil uses Zarf.
	ClusterDeployFn func(context.Context, *layout.PackageLayout, *packager.DeployOptions, bool) error
	// Streams carries operation diagnostics.
	Streams    iostreams.IOStreams
	bundlePath string
}

// ZarfDeployer implements Deployer using the Zarf Go library.
type ZarfDeployer struct {
	streams iostreams.IOStreams
	// Loader overrides source loading for each package when non-nil.
	Loader PackageLayoutLoader
}

// packageStagingRootProvider optionally identifies a directory where package
// staging can be colocated with loader-owned immutable content.
type packageStagingRootProvider interface {
	PackageStagingRoot(context.Context) string
}

// zarfGlobalsOnce guards the one-time, process-wide Zarf configuration applied in NewZarfDeployer.
var zarfGlobalsOnce sync.Once

var _ Deployer = (*ZarfDeployer)(nil)

// DeployPackage invokes the configured package deployment function or base deployer.
func (o *orchestratedDeployer) DeployPackage(ctx context.Context, pkg *spec.Package, opts DeployPackageOptions) error {
	if o.packageDeployFn != nil {
		return o.packageDeployFn(ctx, pkg, opts)
	}
	return o.base.DeployPackage(ctx, pkg, opts)
}

// NewZarfDeployer creates a ZarfDeployer.
// When loader is nil, packages are loaded from their declared source using
// SourcePackageLayoutLoader. For local artifact deploys, provide a
// PackageLayoutLoader implementation that loads packages from the extracted
// artifact's OCI layout instead of pulling from the declared source.
func NewZarfDeployer(streams iostreams.IOStreams, loader PackageLayoutLoader) *ZarfDeployer {
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
func (d *ZarfDeployer) DeployBundle(ctx context.Context, b *spec.UDSBundle, opts DeployOptions) (*DeployResult, error) {
	if err := ValidateConfig(opts.Config); err != nil {
		return nil, err
	}
	if b == nil {
		return nil, NilParameterError{Name: "bundle"}
	}
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("%w for bundle %q: %w", ErrBundleValidation, b.Metadata.Name, err)
	}

	s := logger.Bind(d.streams, opts.Config.Options.LogLevel)

	dag, err := bundleinternal.BuildDependencyGraph(ctx, s, b)
	if err != nil {
		return nil, fmt.Errorf("%w for bundle %q: %w", ErrBuildDependencyGraph, b.Metadata.Name, err)
	}

	levels, err := dag.TopologicalLevels()
	if err != nil {
		return nil, fmt.Errorf("%w for bundle %q: %w", ErrComputeDeploymentLevels, b.Metadata.Name, err)
	}
	s.Debug("dependency graph built", "levels", len(levels))

	// Validate package names before firing the pre-deploy bundle hook, so a hook
	// with side effects does not run for an unknown package. Dependency safety is
	// enforced by the public bundle facade before delegation.
	//
	// Running validation before the hook is contract-safe: the selection is checked
	// against opts.Packages and b.Packages, both fixed before PreDeploy. Per ADR-0013
	// a bundle PreDeploy hook may only mutate
	// opts.PackageDeployHooks and opts.PackageDeployFn (neither feeds package
	// selection), so no contract-conforming hook can change the validation outcome
	// by running first.
	if err := bundleinternal.ValidatePackageNames(opts.Packages, b.Packages); err != nil {
		return nil, err
	}
	if levels, err = bundleinternal.FilterLevels(levels, opts.Packages); err != nil {
		return nil, err
	}

	// Count the packages actually scheduled for deploy (the filtered set), which
	// may be a subset of b.Packages when --packages is used.
	deployCount := 0
	for _, level := range levels {
		deployCount += len(level)
	}

	bhooks := opts.BundleDeployHooks.withDefaults()
	if err := bhooks.PreDeploy(ctx, b, &opts); err != nil {
		return nil, fmt.Errorf("pre-deploy: %w: %w", ErrBundleHook, err)
	}
	if opts.Config == nil || opts.Config.Options == nil {
		return nil, fmt.Errorf("bundle pre-deploy hook left config invalid: %w", ErrBundleHook)
	}
	s = logger.Bind(d.streams, opts.Config.Options.LogLevel)

	concurrency := opts.Config.Options.Concurrency
	bundleDir := filepath.Dir(opts.BundlePath)
	if opts.BundleDir != "" {
		bundleDir = opts.BundleDir
	}
	pkgOpts := DeployPackageOptions{
		Config:             opts.Config,
		BundleDir:          bundleDir,
		PackageDeployHooks: opts.PackageDeployHooks,
		Streams:            s,
		bundlePath:         opts.BundlePath,
	}

	s.Info("deploying bundle", "packages", deployCount, "levels", len(levels), "concurrency", concurrency)

	deployer := &orchestratedDeployer{base: d, packageDeployFn: opts.PackageDeployFn}

	orch := newDeployOrchestrator(deployer, dag, levels, concurrency, pkgOpts, s)
	if err := orch.Run(ctx); err != nil {
		return nil, err
	}
	deployed := orch.DeployedPackages()

	result := &DeployResult{
		BundleName: b.Metadata.Name,
		Packages:   deployed,
	}
	if err := bhooks.PostDeploy(ctx, b); err != nil {
		// Packages are already deployed at this point; return the populated result
		// alongside the error so callers can distinguish "nothing deployed" from
		// "deployed, but the post-deploy hook failed".
		return result, fmt.Errorf("post-deploy: %w: %w", ErrBundleHook, err)
	}
	return result, nil
}

// DeployPackage deploys a single Zarf package using the Zarf Go library.
func (d *ZarfDeployer) DeployPackage(ctx context.Context, pkg *spec.Package, opts DeployPackageOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if pkg == nil {
		return NilParameterError{Name: "package"}
	}
	log := logger.Bind(d.streams, opts.Config.Options.LogLevel)
	log.Debug("preparing package deployment", "name", pkg.Name, "source", pkg.Source)

	ctx = newZarfLoggerContext(ctx, log)

	loader := d.Loader
	if loader == nil {
		loader = &SourcePackageLayoutLoader{configOpts: *opts.Config.Options, bundleDir: opts.BundleDir}
	}
	tmpDir := opts.Config.Options.TmpDir
	stagingRoot := ""
	if provider, ok := loader.(packageStagingRootProvider); ok {
		// Artifact loaders colocate staging with OCI blobs so immutable layers
		// can be hard-linked instead of copied.
		if root := provider.PackageStagingRoot(ctx); root != "" {
			tmpDir = root
			stagingRoot = root
		}
	}
	pkgTmp, err := os.MkdirTemp(tmpDir, "zarf-pkg-*")
	colocatedStaging := err == nil && stagingRoot != ""
	if err != nil && stagingRoot != "" {
		log.Debug("unable to create colocated package staging directory; using configured temporary directory", "dir", stagingRoot, "error", err)
		pkgTmp, err = os.MkdirTemp(opts.Config.Options.TmpDir, "zarf-pkg-*")
	}
	if err != nil {
		return fmt.Errorf("%w for package %q under %q: %w", ErrCreateTemporaryDirectory, pkg.Name, opts.Config.Options.TmpDir, err)
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

	loadResult, err := loader.LoadPackageLayout(ctx, pkg, pkgTmp, LoadOptions{Streams: log, IsPartial: opts.IsPartial})
	if err != nil && colocatedStaging && isRetryableStagingError(err) {
		// A cache can be writable but too small or unsuitable for staging.
		// Retry once in the user-configured temporary directory before failing.
		if removeErr := os.RemoveAll(pkgTmp); removeErr != nil {
			return fmt.Errorf("removing failed colocated package staging directory: %w", removeErr)
		}
		pkgTmp, err = os.MkdirTemp(opts.Config.Options.TmpDir, "zarf-pkg-*")
		if err != nil {
			return fmt.Errorf("creating fallback temp directory: %w", err)
		}
		log.Debug("retrying package staging in configured temporary directory", "dir", opts.Config.Options.TmpDir)
		loadResult, err = loader.LoadPackageLayout(ctx, pkg, pkgTmp, LoadOptions{Streams: log, IsPartial: opts.IsPartial})
	}
	if err != nil {
		return err
	}
	if loadResult == nil {
		return fmt.Errorf("package layout loader returned a nil result for package %q: %w", pkg.Name, ErrLoadPackage)
	}
	pkgLayout := &loadResult.Layout
	opts.IsPartial = loadResult.IsPartial
	defer func() {
		if err := pkgLayout.Cleanup(); err != nil {
			log.Warn("failed to clean up package layout", "name", pkg.Name, "error", err)
		}
	}()

	deployOpts := packager.DeployOptions{
		Values:            zarfValues, // Helm chart values from values_files
		SetVariables:      setVars,    // Zarf ###ZARF_PKG_VAR_*### passthrough
		IsInteractive:     false,
		NamespaceOverride: pkg.Namespace, // empty string is fine - Zarf ignores it
	}

	log.Info("deploying zarf package to cluster", "name", pkg.Name)

	hooks := opts.PackageDeployHooks.withDefaults()
	if err := hooks.PreDeploy(ctx, pkg, pkgLayout, &deployOpts, &opts); err != nil {
		return fmt.Errorf("pre-deploy package %q: %w: %w", pkg.Name, ErrPackageHook, err)
	}

	deploy := opts.ClusterDeployFn
	if deploy == nil {
		deploy = func(ctx context.Context, l *layout.PackageLayout, o *packager.DeployOptions, _ bool) error {
			_, err := packager.Deploy(ctx, l, *o)
			return err
		}
	}
	if err := deploy(ctx, pkgLayout, &deployOpts, opts.IsPartial); err != nil {
		return fmt.Errorf("package %q: %w: %w", pkg.Name, ErrDeployPackage, err)
	}

	if err := hooks.PostDeploy(ctx, pkg); err != nil {
		return fmt.Errorf("post-deploy package %q: %w: %w", pkg.Name, ErrPackageHook, err)
	}

	log.Info("package deployed", "name", pkg.Name)
	return nil
}

func isRetryableStagingError(err error) bool {
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)
}

// prepareValuesAndVariables resolves, templates, and parses values_files for a package,
// and flattens config variables for Zarf ###ZARF_PKG_VAR_*### substitution.
// Temporary files created during templating are cleaned up before this method returns,
// since value.ParseFiles reads them into memory before returning.
func (d *ZarfDeployer) prepareValuesAndVariables(ctx context.Context, streams iostreams.IOStreams, pkg *spec.Package, opts DeployPackageOptions) (zarfValues value.Values, setVars map[string]string, err error) {
	var configVars bundleinternal.Variables
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
			return nil, nil, fmt.Errorf("failed to template values files for package %q: %w: %w", pkg.Name, ErrTemplateValues, err)
		}

		// 3. Parse YAML files with Zarf's value package
		zarfValues, err = value.ParseFiles(ctx, filesToParse, value.ParseFilesOptions{})
		if err != nil {
			return nil, nil, fmt.Errorf("package %q: %w: %w", pkg.Name, ErrParseValues, err)
		}
		loadedFileCount = len(filesToParse)
	}

	// Flatten top-level scalar variables for Zarf ###ZARF_PKG_VAR_*### substitution.
	// Non-scalars are skipped here and flow through values_files instead.
	setVars = configVars.Flatten()

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
func templateValuesFiles(_ context.Context, files []string, vars bundleinternal.Variables, tmpDir string) ([]string, error) {
	if vars == nil {
		return files, nil
	}

	// bundleinternal.Variables is map[string]any underneath, and Go templates traverse named map
	// types via reflection at any depth, so no conversion of nested levels is needed.
	data := map[string]any{"vars": vars}
	result := make([]string, 0, len(files))

	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return result, fmt.Errorf("%w %q: %w", ErrReadValuesFile, f, err)
		}

		tmpl, err := template.New(filepath.Base(f)).Option("missingkey=error").Parse(string(src))
		if err != nil {
			return result, fmt.Errorf("%w %q: %w", ErrParseValuesTemplate, f, err)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return result, fmt.Errorf("%w %q: %w", ErrRenderValues, f, err)
		}

		tmp, err := os.CreateTemp(tmpDir, "uds-values-*.yaml")
		if err != nil {
			return result, fmt.Errorf("%w for %q under %q: %w", ErrCreateTemporaryValuesFile, f, tmpDir, err)
		}
		// Append immediately so the caller's cleanup catches the file even if Write/Close fails.
		result = append(result, tmp.Name())
		if _, err := tmp.Write(buf.Bytes()); err != nil {
			_ = tmp.Close()
			return result, fmt.Errorf("%w %q for %q: %w", ErrWriteTemporaryValuesFile, tmp.Name(), f, err)
		}
		if err := tmp.Close(); err != nil {
			return result, fmt.Errorf("%w %q for %q: %w", ErrCloseTemporaryValuesFile, tmp.Name(), f, err)
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
