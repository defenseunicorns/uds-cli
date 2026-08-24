// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle implements UDS bundle deployment functionality.
package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	internalzarf "github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// DeployPackageOptions contains package deployment context passed to hooks.
type DeployPackageOptions struct {
	// Config supplies variables, temporary-directory settings, and logging
	// configuration. Deploy populates it from DeployOptions.Config.
	Config             *UDSBundleConfig
	BundleDir          string
	PackageDeployHooks PackageDeployHooks
	// IsPartial reports whether the loaded layout omits checksum-referenced layers.
	IsPartial bool
	Streams   iostreams.IOStreams
}

// ZarfPackageLayoutLoadOptions carries options for loading a Zarf package layout.
type ZarfPackageLayoutLoadOptions struct {
	Streams   iostreams.IOStreams
	IsPartial bool
}

// ZarfPackageLayoutLoader prepares a Zarf package layout for a bundle package.
// Implementations may pull from pkg.Source, load from an extracted bundle
// artifact, or return an already-staged layout.
type ZarfPackageLayoutLoader interface {
	LoadPackageLayout(ctx context.Context, pkg *spec.Package, dstDir string, opts ZarfPackageLayoutLoadOptions) (*ZarfPackageLayoutLoadResult, error)
}

// ZarfPackageLayoutLoadResult contains a loaded layout and metadata discovered while loading it.
type ZarfPackageLayoutLoadResult struct {
	Layout    ZarfPackageLayout
	IsPartial bool
}

// PackageDeployHooks provides deployment process extensibility on a per-package basis.
type PackageDeployHooks struct {
	// PreDeploy enables customization just before a package deploys. It runs after
	// layout loading and before the cluster deploy. Mutations to pkgLayout.Pkg and
	// packageOpts take effect immediately. A non-nil error aborts the deploy; the
	// cluster deploy is not called and PostDeploy is skipped.
	//
	// PreDeploy and PostDeploy are captured before PreDeploy runs, so changing
	// packageOpts.PackageDeployHooks here has no effect. Use
	// BundleDeployHooks.PreDeploy to install per-package hooks dynamically.
	//
	// PreDeploy may run concurrently with PreDeploy for other packages in the
	// same DAG level; implementations must be concurrency-safe.
	PreDeploy func(ctx context.Context, pkg *spec.Package, pkgLayout *ZarfPackageLayout, packageOpts *DeployPackageOptions) error

	// PostDeploy enables tracking successfully deployed packages. It runs after a
	// successful cluster deploy and is not called when PreDeploy or deployment
	// returns an error. It may run concurrently with PostDeploy for other
	// packages in the same DAG level; implementations must be concurrency-safe.
	PostDeploy func(ctx context.Context, pkg *spec.Package) error
}

// BundleDeployHooks provides deployment process extensibility at the bundle scope.
type BundleDeployHooks struct {
	// PreDeploy runs once before package deployment. Only mutations to
	// PackageDeployHooks are honored: package selection, source preparation, and
	// bundle hooks have already been consumed. A returned error prevents package
	// deployment and skips PostDeploy.
	PreDeploy func(ctx context.Context, b *spec.UDSBundle, opts *DeployOptions) error
	// PostDeploy runs once after every selected package deploys successfully.
	PostDeploy func(ctx context.Context, b *spec.UDSBundle) error
}

// DeploySource abstracts the bundle definition, optional parsed bundle, and
// source-specific package loading behavior. It owns temporary resources created
// while preparing an artifact source.
type DeploySource struct {
	// BundlePath is the absolute path to the bundle definition file (bundle.uds.hcl).
	BundlePath string
	// DefaultsPath is an optional defaults.uds.hcl path associated with the source.
	DefaultsPath string
	// Bundle is an optional parsed bundle. Prepared artifact sources populate it
	// with deploy-ready values file paths; nil means Deploy parses BundlePath.
	Bundle *spec.UDSBundle
	// Loader overrides how package layouts are obtained; nil means use the default source loader.
	Loader ZarfPackageLayoutLoader

	close func() error
}

// Close releases any temporary resources allocated during source preparation.
func (s *DeploySource) Close() error {
	if s == nil || s.close == nil {
		return nil
	}
	return s.close()
}

// DeployOptions contains options for deploying an entire bundle.
type DeployOptions struct {
	Config   *UDSBundleConfig
	Packages []string
	// Force bypasses ValidateDeploySafety, allowing selected packages to deploy
	// even when required dependencies are absent.
	Force              bool
	BundleDeployHooks  BundleDeployHooks
	PackageDeployHooks PackageDeployHooks
	Streams            iostreams.IOStreams
}

// DeployResult represents the output of a bundle deploy operation.
type DeployResult struct {
	BundleName string                `json:"bundleName" yaml:"bundleName" text:"Bundle Name"`
	Packages   []DeployPackageResult `json:"packages" yaml:"packages" text:"Packages"`
}

// DeployPackageResult represents a package successfully deployed as part of a bundle.
type DeployPackageResult struct {
	Name string `json:"name" yaml:"name" text:"Name"`
}

// Deploy deploys a UDS bundle to a Kubernetes cluster.
// It delegates bundle-level deployment (DAG traversal, ordering, parallelism,
// and concurrency limits) to the deployment adapter.
func Deploy(ctx context.Context, source *DeploySource, opts DeployOptions) (*DeployResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if source == nil {
		return nil, fmt.Errorf("source is required: %w", ErrSourceRequired)
	}
	if source.BundlePath == "" && source.Bundle == nil {
		return nil, fmt.Errorf("source must provide BundlePath or Bundle: %w", ErrBundleInputRequired)
	}
	if source.DefaultsPath != "" {
		config, err := applyEmbeddedDefaults(ctx, opts.Config, source.DefaultsPath, source.Loader != nil)
		if err != nil {
			return nil, fmt.Errorf("%w: applying embedded defaults: %w", ErrDeployBundle, err)
		}
		opts.Config = config
	}

	s := logger.Bind(opts.Streams, opts.Config.Options.LogLevel)

	b := source.Bundle
	if b == nil {
		s.Debug("parsing bundle", "path", source.BundlePath)
		var err error
		if source.Loader != nil {
			var bundleBytes []byte
			bundleBytes, err = os.ReadFile(source.BundlePath)
			if err == nil {
				b, err = bundleinternal.NewHCLParser(opts.Config.Options.Architecture, s).ParseBundleBytes(ctx, bundleBytes)
			}
		} else {
			b, err = parseBundleFile(ctx, opts.Config.Options.Architecture, s, source.BundlePath)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: failed to parse bundle: %w", ErrDeployBundle, err)
		}
		s.Debug("bundle parsed", "name", b.Metadata.Name, "packages", len(b.Packages))
	}
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("%w: bundle validation failed: %w", ErrDeployBundle, err)
	}
	if !opts.Force {
		if err := validateDeploySafety(ctx, s, b, opts.Packages); err != nil {
			return nil, fmt.Errorf("%w: unable to deploy safely: %w", ErrDeployBundle, err)
		}
	}

	deployer := newZarfDeployer(s, source.Loader)
	result, err := deployer.deployBundle(ctx, b, opts, source)
	if err != nil {
		return result, fmt.Errorf("%w %q: %w", ErrDeployBundle, b.Metadata.Name, err)
	}
	if result == nil {
		return nil, fmt.Errorf("%w: deployer returned no result", ErrDeployBundle)
	}
	s.Info("bundle deployed", "name", result.BundleName, "packages", len(result.Packages))
	return result, nil
}

type zarfDeployer struct {
	deployer *internalzarf.ZarfDeployer
	streams  iostreams.IOStreams
}

func newZarfDeployer(streams iostreams.IOStreams, loader ZarfPackageLayoutLoader) *zarfDeployer {
	var internalLoader internalzarf.PackageLayoutLoader
	if loader != nil {
		internalLoader = packageLayoutLoaderAdapter{loader: loader}
	}
	return &zarfDeployer{deployer: internalzarf.NewZarfDeployer(streams, internalLoader), streams: streams}
}

func (d *zarfDeployer) deployBundle(ctx context.Context, b *spec.UDSBundle, opts DeployOptions, source *DeploySource) (*DeployResult, error) {
	if err := validateDirectDeployOptions(opts, b); err != nil {
		return nil, err
	}
	if source == nil && !opts.Force {
		if err := validateDeploySafety(ctx, d.streams, b, opts.Packages); err != nil {
			return nil, err
		}
	}
	result, err := d.deployer.DeployBundle(ctx, b, toZarfDeployOptions(opts, source))
	if result == nil {
		return nil, err
	}
	packages := make([]DeployPackageResult, len(result.Packages))
	for i, name := range result.Packages {
		packages[i] = DeployPackageResult{Name: name}
	}
	return &DeployResult{BundleName: result.BundleName, Packages: packages}, err
}

func validateDirectDeployOptions(opts DeployOptions, b *spec.UDSBundle) error {
	if err := validateConfig(opts.Config); err != nil {
		return err
	}
	if b == nil {
		return fmt.Errorf("bundle is required")
	}
	return nil
}

func toZarfConfig(cfg *UDSBundleConfig) *internalzarf.UDSBundleConfig {
	if cfg == nil {
		return nil
	}
	var options *bundleinternal.ConfigOptions
	if cfg.Options != nil {
		options = &bundleinternal.ConfigOptions{
			LogLevel: cfg.Options.LogLevel, Architecture: cfg.Options.Architecture,
			PlainHTTP: cfg.Options.PlainHTTP, SkipTLSVerify: cfg.Options.SkipTLSVerify,
			TmpDir: cfg.Options.TmpDir, Concurrency: cfg.Options.Concurrency,
		}
	}
	return &internalzarf.UDSBundleConfig{Options: options, Variables: bundleinternal.Variables(cfg.Variables)}
}

func fromZarfConfig(cfg *internalzarf.UDSBundleConfig) *UDSBundleConfig {
	if cfg == nil {
		return nil
	}
	var options *ConfigOptions
	if cfg.Options != nil {
		options = &ConfigOptions{
			LogLevel: cfg.Options.LogLevel, Architecture: cfg.Options.Architecture,
			PlainHTTP: cfg.Options.PlainHTTP, SkipTLSVerify: cfg.Options.SkipTLSVerify,
			TmpDir: cfg.Options.TmpDir, Concurrency: cfg.Options.Concurrency,
		}
	}
	return &UDSBundleConfig{Options: options, Variables: Variables(cfg.Variables)}
}

func toZarfDeployPackageOptions(opts DeployPackageOptions) internalzarf.DeployPackageOptions {
	return internalzarf.DeployPackageOptions{
		Config:             toZarfConfig(opts.Config),
		BundleDir:          opts.BundleDir,
		PackageDeployHooks: toZarfPackageHooks(opts.PackageDeployHooks),
		IsPartial:          opts.IsPartial,
		Streams:            opts.Streams,
	}
}

func fromZarfDeployPackageOptions(opts internalzarf.DeployPackageOptions) DeployPackageOptions {
	return DeployPackageOptions{
		Config:    fromZarfConfig(opts.Config),
		BundleDir: opts.BundleDir,
		IsPartial: opts.IsPartial,
		Streams:   opts.Streams,
	}
}

func toZarfPackageHooks(hooks PackageDeployHooks) internalzarf.PackageDeployHooks {
	var result internalzarf.PackageDeployHooks
	if hooks.PreDeploy != nil {
		result.PreDeploy = func(ctx context.Context, pkg *spec.Package, pkgLayout *layout.PackageLayout, _ *packager.DeployOptions, internalOpts *internalzarf.DeployPackageOptions) error {
			publicOpts := fromZarfDeployPackageOptions(*internalOpts)
			publicOpts.PackageDeployHooks = hooks
			publicLayout := fromZarfPackageLayout(pkgLayout)
			err := hooks.PreDeploy(ctx, pkg, publicLayout, &publicOpts)
			if applyErr := applyPublicPackageLayout(pkgLayout, publicLayout); applyErr != nil {
				return applyErr
			}
			if err != nil {
				return err
			}
			if internalOpts.Config != nil {
				if configErr := validateConfig(publicOpts.Config); configErr != nil {
					return configErr
				}
			}
			internalOpts.Config = toZarfConfig(publicOpts.Config)
			internalOpts.BundleDir = publicOpts.BundleDir
			internalOpts.PackageDeployHooks = toZarfPackageHooks(publicOpts.PackageDeployHooks)
			internalOpts.IsPartial = publicOpts.IsPartial
			internalOpts.Streams = publicOpts.Streams
			return nil
		}
	}
	result.PostDeploy = hooks.PostDeploy
	return result
}

func toZarfDeployOptions(opts DeployOptions, source *DeploySource) internalzarf.DeployOptions {
	bundlePath := ""
	bundleDir := ""
	if source != nil {
		bundlePath = source.BundlePath
		bundleDir = filepath.Dir(source.BundlePath)
	}
	internal := internalzarf.DeployOptions{
		Config:             toZarfConfig(opts.Config),
		BundlePath:         bundlePath,
		BundleDir:          bundleDir,
		Packages:           opts.Packages,
		PackageDeployHooks: toZarfPackageHooks(opts.PackageDeployHooks),
	}
	if opts.BundleDeployHooks.PreDeploy != nil {
		internal.BundleDeployHooks.PreDeploy = func(ctx context.Context, b *spec.UDSBundle, internalOpts *internalzarf.DeployOptions) error {
			publicOpts := opts
			publicOpts.Config = fromZarfConfig(internalOpts.Config)
			publicOpts.Packages = internalOpts.Packages
			if err := opts.BundleDeployHooks.PreDeploy(ctx, b, &publicOpts); err != nil {
				return err
			}
			internalOpts.Config = toZarfConfig(publicOpts.Config)
			internalOpts.PackageDeployHooks = toZarfPackageHooks(publicOpts.PackageDeployHooks)
			return nil
		}
	}
	internal.BundleDeployHooks.PostDeploy = opts.BundleDeployHooks.PostDeploy
	return internal
}

func applyEmbeddedDefaults(ctx context.Context, config *UDSBundleConfig, defaultsPath string, artifactSource bool) (*UDSBundleConfig, error) {
	var (
		defaults bundleinternal.Variables
		err      error
	)
	if config == nil {
		return nil, fmt.Errorf("config must not be nil")
	}
	if !artifactSource {
		defaults, err = bundleinternal.ParseDefaults(ctx, defaultsPath)
		if err != nil {
			return nil, fmt.Errorf("loading embedded defaults: %w", err)
		}
	} else {
		defaultsData, err := os.ReadFile(defaultsPath)
		if err != nil {
			return nil, fmt.Errorf("loading embedded defaults: %w", err)
		}
		defaults, err = bundleinternal.ParseDefaultsBytes(ctx, defaultsData)
		if err != nil {
			return nil, fmt.Errorf("loading embedded defaults: %w", err)
		}
	}
	merged := *config
	merged.Variables = fromInternalVariables(bundleinternal.MergeVariables(bundleinternal.Variables(defaults), toInternalVariables(config.Variables)))
	return &merged, nil
}

func fromInternalVariables(variables bundleinternal.Variables) Variables {
	if variables == nil {
		return nil
	}
	result := make(Variables, len(variables))
	for key, value := range variables {
		result[key] = fromInternalVariableValue(value)
	}
	return result
}

func fromInternalVariableValue(value any) any {
	switch value := value.(type) {
	case bundleinternal.Variables:
		return fromInternalVariables(value)
	case map[string]any:
		return fromInternalVariables(bundleinternal.Variables(value))
	case []any:
		result := make([]any, len(value))
		for i, item := range value {
			result[i] = fromInternalVariableValue(item)
		}
		return result
	default:
		return value
	}
}

// PrepareDeploySource prepares a bundle directory or verified tar.zst artifact.
func PrepareDeploySource(ctx context.Context, streams iostreams.IOStreams, path, tmpDir, architecture string) (*DeploySource, error) {
	if path == "" {
		return nil, fmt.Errorf("path must not be empty: %w", ErrSourceRequired)
	}
	if !artifact.IsTarZst(path) {
		bundlePath := bundleinternal.ResolveBundlePath(path)
		defaultsPath, err := bundleinternal.AdjacentDefaultsPath(filepath.Dir(bundlePath))
		if err != nil {
			return nil, fmt.Errorf("%w: discovering adjacent defaults: %w", ErrPrepareDeploySource, err)
		}
		return &DeploySource{BundlePath: bundlePath, DefaultsPath: defaultsPath}, nil
	}

	workspaceDir, err := os.MkdirTemp(tmpDir, "uds-bundle-deploy-*")
	if err != nil {
		return nil, fmt.Errorf("%w: creating workspace for bundle artifact: %w", ErrPrepareDeploySource, err)
	}
	cleanup := func() error { return os.RemoveAll(workspaceDir) }
	extracted, err := artifact.ExtractArtifact(ctx, streams, path, workspaceDir)
	if err != nil {
		_ = cleanup()
		return nil, fmt.Errorf("%w: extracting bundle artifact: %w", ErrPrepareDeploySource, err)
	}
	valuesOverride, err := extracted.ValuesFilesByPackage()
	if err != nil {
		_ = cleanup()
		return nil, fmt.Errorf("%w: collecting values files from artifact: %w", ErrPrepareDeploySource, err)
	}

	bundleBytes, err := os.ReadFile(extracted.BundleDefPath)
	if err != nil {
		_ = cleanup()
		return nil, fmt.Errorf("%w: reading extracted bundle definition: %w", ErrPrepareDeploySource, err)
	}
	preparedBundle, err := bundleinternal.NewHCLParser(architecture, streams).ParseBundleBytes(ctx, bundleBytes)
	if err != nil {
		_ = cleanup()
		return nil, fmt.Errorf("%w: parsing extracted bundle definition: %w", ErrPrepareDeploySource, err)
	}
	if err := applyArtifactValuesFiles(preparedBundle, valuesOverride, extracted.Dir); err != nil {
		_ = cleanup()
		return nil, fmt.Errorf("%w from %q: %w", ErrPrepareDeploySource, path, err)
	}

	source := &DeploySource{
		BundlePath: extracted.BundleDefPath,
		Bundle:     preparedBundle,
		Loader: &extractedArtifactPackageLayoutLoader{loader: &internalzarf.ExtractedArtifactPackageLayoutLoader{
			OCIDir: extracted.OCIDir, PackageManifests: extracted.PackageManifests,
		}},
		close: cleanup,
	}
	source.DefaultsPath, err = bundleinternal.AdjacentDefaultsPath(filepath.Dir(extracted.BundleDefPath))
	if err != nil {
		_ = cleanup()
		return nil, fmt.Errorf("%w: discovering adjacent defaults: %w", ErrPrepareDeploySource, err)
	}
	return source, nil
}

func applyArtifactValuesFiles(b *spec.UDSBundle, valuesByPackage map[string][]string, bundleDir string) error {
	for i := range b.Packages {
		paths := valuesByPackage[b.Packages[i].Name]
		b.Packages[i].ValuesFiles = nil
		if paths == nil {
			continue
		}
		b.Packages[i].ValuesFiles = make([]string, len(paths))
		for j, path := range paths {
			var err error
			b.Packages[i].ValuesFiles[j], err = filepath.Rel(bundleDir, path)
			if err != nil {
				return fmt.Errorf("relating extracted values file path: %w", err)
			}
		}
	}
	return nil
}
