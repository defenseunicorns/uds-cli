// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"errors"

	"github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// ExtractedArtifactPackageLayoutLoader loads packages from an extracted bundle artifact.
type ExtractedArtifactPackageLayoutLoader struct {
	OCIDir           string
	PackageDigests   map[string]string
	packageManifests map[string]ocispec.Descriptor
}

// LoadPackageLayout stages an extracted package as a deployable layout.
func (l *ExtractedArtifactPackageLayoutLoader) LoadPackageLayout(ctx context.Context, pkg *spec.Package, dstDir string, opts LoadOptions) (*layout.PackageLayout, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	loader := &zarf.ExtractedArtifactPackageLayoutLoader{OCIDir: l.OCIDir, PackageDigests: l.PackageDigests, PackageManifests: l.packageManifests}
	return loader.LoadPackageLayout(ctx, pkg, dstDir, zarf.LoadOptions{Streams: opts.Streams, IsPartial: opts.IsPartial})
}

// ZarfDeployer is the public adapter over the private Zarf deployer.
type ZarfDeployer struct {
	deployer *zarf.ZarfDeployer
	streams  iostreams.IOStreams
}

// NewZarfDeployer creates a Zarf-backed deployer.
func NewZarfDeployer(streams iostreams.IOStreams, loader PackageLayoutLoader) *ZarfDeployer {
	var internalLoader zarf.PackageLayoutLoader
	if loader != nil {
		internalLoader = packageLayoutLoaderAdapter{loader: loader}
	}
	return &ZarfDeployer{deployer: zarf.NewZarfDeployer(streams, internalLoader), streams: streams}
}

// DeployPackage deploys one package through the private Zarf integration.
func (d *ZarfDeployer) DeployPackage(ctx context.Context, pkg *spec.Package, opts DeployPackageOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	return d.deployer.DeployPackage(ctx, pkg, toZarfDeployPackageOptions(opts))
}

// DeployBundle deploys a bundle through the private Zarf integration.
func (d *ZarfDeployer) DeployBundle(ctx context.Context, b *spec.UDSBundle, opts DeployOptions) (*DeployResult, error) {
	if err := validateDirectDeployOptions(opts, b); err != nil {
		return nil, err
	}
	if !opts.Force && b != nil {
		if err := ValidateDeploySafety(ctx, d.streams, b, opts.Packages); err != nil {
			return nil, err
		}
	}
	result, err := d.deployer.DeployBundle(ctx, b, toZarfDeployOptions(opts))
	if result == nil {
		return nil, err
	}
	return &DeployResult{BundleName: result.BundleName, Packages: result.Packages}, err
}

// ZarfRemover is the public adapter over the private Zarf remover.
type ZarfRemover struct {
	remover *zarf.ZarfRemover
	streams iostreams.IOStreams
}

// NewZarfRemover creates a Zarf-backed remover.
func NewZarfRemover(streams iostreams.IOStreams) *ZarfRemover {
	return &ZarfRemover{remover: zarf.NewZarfRemover(streams), streams: streams}
}

// RemovePackage removes one package through the private Zarf integration.
func (r *ZarfRemover) RemovePackage(ctx context.Context, pkg *spec.Package, opts RemovePackageOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	err := r.remover.RemovePackage(ctx, pkg, zarf.RemovePackageOptions{Config: toZarfConfig(opts.Config), Force: opts.Force})
	if errors.Is(err, zarf.ErrPackageNotDeployed) {
		return ErrPackageNotDeployed
	}
	return err
}

// RemoveBundle removes a bundle through the private Zarf integration.
func (r *ZarfRemover) RemoveBundle(ctx context.Context, b *spec.UDSBundle, packages []string, opts RemovePackageOptions) (*RemoveResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if !opts.Force && b != nil {
		if err := ValidateRemovalSafety(ctx, r.streams, b, packages); err != nil {
			return nil, err
		}
	}
	result, err := r.remover.RemoveBundle(ctx, b, packages, zarf.RemovePackageOptions{Config: toZarfConfig(opts.Config), Force: opts.Force})
	if result == nil {
		return nil, err
	}
	return &RemoveResult{BundleName: result.BundleName, Removed: result.Removed, Skipped: result.Skipped}, err
}

// validateDirectDeployOptions applies the bundle argument to the option contract
// used by the top-level Deploy entrypoint.
func validateDirectDeployOptions(opts DeployOptions, b *spec.UDSBundle) error {
	opts.Bundle = b
	return opts.Validate()
}

// packageLayoutLoaderAdapter converts internal loader options for a public loader.
type packageLayoutLoaderAdapter struct {
	loader PackageLayoutLoader
}

// LoadPackageLayout delegates package loading through the public loader contract.
func (a packageLayoutLoaderAdapter) LoadPackageLayout(ctx context.Context, pkg *spec.Package, dstDir string, opts zarf.LoadOptions) (*layout.PackageLayout, error) {
	return a.loader.LoadPackageLayout(ctx, pkg, dstDir, LoadOptions{Streams: opts.Streams, IsPartial: opts.IsPartial})
}

// toZarfConfig converts public bundle configuration to private Zarf configuration.
func toZarfConfig(cfg *UDSBundleConfig) *zarf.UDSBundleConfig {
	if cfg == nil {
		return nil
	}
	var global *zarf.GlobalOptions
	if cfg.Global != nil {
		global = &zarf.GlobalOptions{LogLevel: cfg.Global.LogLevel, Prompt: cfg.Global.Prompt}
	}
	var options *zarf.ConfigOptions
	if cfg.Options != nil {
		options = &zarf.ConfigOptions{
			LogLevel: cfg.Options.LogLevel, Architecture: cfg.Options.Architecture,
			PlainHTTP: cfg.Options.PlainHTTP, SkipTLSVerify: cfg.Options.SkipTLSVerify,
			UDSCache: cfg.Options.UDSCache, TmpDir: cfg.Options.TmpDir, Concurrency: cfg.Options.Concurrency,
		}
	}
	return &zarf.UDSBundleConfig{Global: global, Options: options, Variables: zarf.Variables(cfg.Variables), Remain: cfg.Remain}
}

// toZarfDeployPackageOptions converts public package deployment options for Zarf.
func toZarfDeployPackageOptions(opts DeployPackageOptions) zarf.DeployPackageOptions {
	return zarf.DeployPackageOptions{
		Config:             toZarfConfig(opts.Config),
		BundleDir:          opts.BundleDir,
		PackageDeployHooks: toZarfPackageHooks(opts.PackageDeployHooks),
		ClusterDeployFn:    opts.ClusterDeployFn,
		Streams:            opts.Streams,
	}
}

// fromZarfDeployPackageOptions converts private Zarf package options to public options.
func fromZarfDeployPackageOptions(opts zarf.DeployPackageOptions) DeployPackageOptions {
	return DeployPackageOptions{
		Config:          fromZarfConfig(opts.Config),
		BundleDir:       opts.BundleDir,
		ClusterDeployFn: opts.ClusterDeployFn,
		Streams:         opts.Streams,
	}
}

// toZarfPackageHooks adapts public package hooks to the private Zarf hook contract.
func toZarfPackageHooks(hooks PackageDeployHooks) zarf.PackageDeployHooks {
	var result zarf.PackageDeployHooks
	if hooks.PreDeploy != nil {
		result.PreDeploy = func(ctx context.Context, pkg *spec.Package, pkgLayout *layout.PackageLayout, deployOpts *packager.DeployOptions, internalOpts *zarf.DeployPackageOptions) error {
			publicOpts := fromZarfDeployPackageOptions(*internalOpts)
			err := hooks.PreDeploy(ctx, pkg, pkgLayout, deployOpts, &publicOpts)
			internalOpts.Config = toZarfConfig(publicOpts.Config)
			internalOpts.BundleDir = publicOpts.BundleDir
			internalOpts.ClusterDeployFn = publicOpts.ClusterDeployFn
			internalOpts.Streams = publicOpts.Streams
			return err
		}
	}
	result.PostDeploy = hooks.PostDeploy
	return result
}

// toZarfDeployOptions converts public bundle deployment options for Zarf.
func toZarfDeployOptions(opts DeployOptions) zarf.DeployOptions {
	internal := zarf.DeployOptions{
		Config:             toZarfConfig(opts.Config),
		BundlePath:         opts.BundlePath,
		Packages:           opts.Packages,
		PackageDeployHooks: toZarfPackageHooks(opts.PackageDeployHooks),
	}
	if opts.PackageDeployFn != nil {
		internal.PackageDeployFn = func(ctx context.Context, pkg *spec.Package, packageOpts zarf.DeployPackageOptions) error {
			publicPackageOpts := fromZarfDeployPackageOptions(packageOpts)
			publicPackageOpts.PackageDeployHooks = opts.PackageDeployHooks
			return opts.PackageDeployFn(ctx, pkg, publicPackageOpts)
		}
	}
	if opts.BundleDeployHooks.PreDeploy != nil {
		internal.BundleDeployHooks.PreDeploy = func(ctx context.Context, b *spec.UDSBundle, internalOpts *zarf.DeployOptions) error {
			publicOpts := opts
			publicOpts.Config = fromZarfConfig(internalOpts.Config)
			publicOpts.BundlePath = internalOpts.BundlePath
			publicOpts.Packages = internalOpts.Packages
			if err := opts.BundleDeployHooks.PreDeploy(ctx, b, &publicOpts); err != nil {
				return err
			}
			internalOpts.PackageDeployHooks = toZarfPackageHooks(publicOpts.PackageDeployHooks)
			if publicOpts.PackageDeployFn != nil {
				internalOpts.PackageDeployFn = func(ctx context.Context, pkg *spec.Package, packageOpts zarf.DeployPackageOptions) error {
					publicPackageOpts := fromZarfDeployPackageOptions(packageOpts)
					publicPackageOpts.PackageDeployHooks = publicOpts.PackageDeployHooks
					return publicOpts.PackageDeployFn(ctx, pkg, publicPackageOpts)
				}
			} else {
				internalOpts.PackageDeployFn = nil
			}
			return nil
		}
	}
	internal.BundleDeployHooks.PostDeploy = opts.BundleDeployHooks.PostDeploy
	return internal
}

// fromZarfConfig converts private Zarf configuration to public bundle configuration.
func fromZarfConfig(cfg *zarf.UDSBundleConfig) *UDSBundleConfig {
	if cfg == nil {
		return nil
	}
	var global *GlobalOptions
	if cfg.Global != nil {
		global = &GlobalOptions{LogLevel: cfg.Global.LogLevel, Prompt: cfg.Global.Prompt}
	}
	var options *ConfigOptions
	if cfg.Options != nil {
		options = &ConfigOptions{
			LogLevel: cfg.Options.LogLevel, Architecture: cfg.Options.Architecture,
			PlainHTTP: cfg.Options.PlainHTTP, SkipTLSVerify: cfg.Options.SkipTLSVerify,
			UDSCache: cfg.Options.UDSCache, TmpDir: cfg.Options.TmpDir, Concurrency: cfg.Options.Concurrency,
		}
	}
	return &UDSBundleConfig{Global: global, Options: options, Variables: Variables(cfg.Variables), Remain: cfg.Remain}
}
