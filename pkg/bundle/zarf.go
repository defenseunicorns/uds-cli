// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// ZarfPackageLayout exposes the native Zarf package definition during bundle
// deploy. Keeping the schema-aware definition intact lets hooks mutate fields
// specific to the package API version in use.
type ZarfPackageLayout struct {
	dirPath           string
	PackageDefinition api.PackageDefinition
	digest            string
}

// SetDeployedDigest records the registry-resolved manifest digest that Zarf
// should store as the deployed package identity.
func (p *ZarfPackageLayout) SetDeployedDigest(digest string) {
	if p != nil {
		p.digest = digest
	}
}

func (p *ZarfPackageLayout) DirPath() string {
	return p.dirPath
}

func (p *ZarfPackageLayout) Digest() string {
	return p.digest
}

type extractedArtifactPackageLayoutLoader struct {
	loader *zarf.ExtractedArtifactPackageLayoutLoader
}

// PackageStagingRootProvider optionally identifies a directory where package
// staging can be colocated with loader-owned immutable content. An empty return
// value uses the configured temporary directory.
type PackageStagingRootProvider interface {
	PackageStagingRoot(context.Context) string
}

func (l *extractedArtifactPackageLayoutLoader) LoadPackageLayout(ctx context.Context, pkg *spec.Package, dstDir string, opts ZarfPackageLayoutLoadOptions) (*ZarfPackageLayoutLoadResult, error) {
	result, err := l.loader.LoadPackageLayout(ctx, pkg, dstDir, zarf.LoadOptions{Streams: opts.Streams, IsPartial: opts.IsPartial})
	if err != nil {
		return nil, err
	}
	return &ZarfPackageLayoutLoadResult{
		Layout:    *fromZarfPackageLayout(&result.Layout),
		IsPartial: result.IsPartial,
	}, nil
}

// PackageStagingRoot returns the parent of OCIDir so package staging can share
// the artifact workspace and hard-link immutable OCI blobs.
func (l *extractedArtifactPackageLayoutLoader) PackageStagingRoot(_ context.Context) string {
	if l.loader.OCIDir == "" {
		return ""
	}
	return filepath.Dir(filepath.Clean(l.loader.OCIDir))
}

// packageLayoutLoaderAdapter converts internal loader options for a public loader.
type packageLayoutLoaderAdapter struct {
	loader ZarfPackageLayoutLoader
}

// LoadPackageLayout delegates package loading through the public loader contract.
func (a packageLayoutLoaderAdapter) LoadPackageLayout(ctx context.Context, pkg *spec.Package, dstDir string, opts zarf.LoadOptions) (*zarf.PackageLayoutLoadResult, error) {
	publicResult, err := a.loader.LoadPackageLayout(ctx, pkg, dstDir, ZarfPackageLayoutLoadOptions{Streams: opts.Streams, IsPartial: opts.IsPartial})
	if err != nil {
		return nil, err
	}
	if publicResult == nil {
		return nil, fmt.Errorf("package layout loader returned a nil result")
	}

	publicLayout := &publicResult.Layout
	isPartial := publicResult.IsPartial || opts.IsPartial
	stagedDir, err := filepath.Abs(filepath.Clean(dstDir))
	if err != nil {
		return nil, fmt.Errorf("resolving package staging directory %q: %w", dstDir, err)
	}
	// The adapter owns the staging directory. Public loaders populate dstDir but
	// cannot and should not set layout-private path state.
	publicLayout.dirPath = stagedDir
	internalLayout, err := toZarfPackageLayout(ctx, publicLayout, isPartial)
	if err != nil {
		return nil, fmt.Errorf("loading package layout staged in %q: %w", stagedDir, err)
	}
	return &zarf.PackageLayoutLoadResult{
		Layout:    *internalLayout,
		IsPartial: isPartial,
	}, nil
}

// PackageStagingRoot forwards an optional staging preference. An empty result
// leaves staging in the configured temporary directory.
func (a packageLayoutLoaderAdapter) PackageStagingRoot(ctx context.Context) string {
	if provider, ok := a.loader.(PackageStagingRootProvider); ok {
		return provider.PackageStagingRoot(ctx)
	}
	return ""
}

func fromZarfPackageLayout(pkgLayout *layout.PackageLayout) *ZarfPackageLayout {
	if pkgLayout == nil {
		return nil
	}
	result := &ZarfPackageLayout{
		dirPath:           pkgLayout.DirPath(),
		PackageDefinition: pkgLayout.PackageDefinition,
	}
	if !pkgLayout.IsPushable() && pkgLayout.Digest() != "" {
		result.digest = pkgLayout.Digest()
	}
	return result
}

func toZarfPackageLayout(ctx context.Context, pkgLayout *ZarfPackageLayout, isPartial bool) (*layout.PackageLayout, error) {
	if pkgLayout == nil {
		return nil, nil
	}
	if pkgLayout.dirPath != "" {
		result, err := layout.LoadFromDir(ctx, pkgLayout.dirPath, layout.PackageLayoutOptions{
			IsPartial:            isPartial,
			VerificationStrategy: layout.VerifyNever,
		})
		if err != nil {
			return nil, err
		}
		if err := applyPublicPackageLayout(result, pkgLayout); err != nil {
			return nil, err
		}
		return result, nil
	}
	return toZarfPackageLayoutForDeploy(pkgLayout)
}

func toZarfPackageLayoutForDeploy(pkgLayout *ZarfPackageLayout) (*layout.PackageLayout, error) {
	if pkgLayout == nil {
		return nil, nil
	}
	result := &layout.PackageLayout{}
	result.PackageDefinition = pkgLayout.PackageDefinition
	return result, nil
}

func applyPublicPackageLayout(dst *layout.PackageLayout, src *ZarfPackageLayout) error {
	if dst == nil || src == nil {
		return nil
	}
	dst.PackageDefinition = src.PackageDefinition
	if src.digest != "" {
		dst.SetRegistryDigest(src.digest)
	}
	return nil
}
