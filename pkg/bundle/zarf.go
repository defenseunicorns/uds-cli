// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// ZarfPackageLayout is the supported public subset of
// github.com/zarf-dev/zarf/src/pkg/packager/layout.PackageLayout exposed during
// bundle deploy. Fields not exposed here are preserved by the adapter.
type ZarfPackageLayout struct {
	Directory string
	// IsPartial indicates that the layout may omit files referenced by its checksums.
	IsPartial      bool
	Pkg            ZarfPackage
	registryDigest string
}

// SetDeployedDigest records the registry-resolved manifest digest that Zarf
// should store as the deployed package identity.
func (p *ZarfPackageLayout) SetDeployedDigest(digest string) {
	if p != nil {
		p.registryDigest = digest
	}
}

// ZarfPackage is the supported public subset of
// github.com/zarf-dev/zarf/src/api/v1alpha1.ZarfPackage exposed through
// ZarfPackageLayout.
type ZarfPackage struct {
	Components []ZarfPackageComponent
}

// ZarfPackageComponent is the supported public subset of
// github.com/zarf-dev/zarf/src/api/v1alpha1.ZarfComponent exposed to bundle
// deploy hooks.
type ZarfPackageComponent struct {
	Name          string
	Images        []string
	ImageArchives []ZarfPackageImageArchive
	privateID     string
}

// ZarfPackageImageArchive is the supported public subset of
// github.com/zarf-dev/zarf/src/api/v1alpha1.ImageArchive exposed through
// ZarfPackageComponent.
type ZarfPackageImageArchive struct {
	Path   string
	Images []string
}

type extractedArtifactPackageLayoutLoader struct {
	loader *zarf.ExtractedArtifactPackageLayoutLoader
}

func (l *extractedArtifactPackageLayoutLoader) LoadPackageLayout(ctx context.Context, pkg *spec.Package, dstDir string, opts ZarfPackageLayoutLoadOptions) (*ZarfPackageLayout, error) {
	pkgLayout, isPartial, err := l.loader.LoadPackageLayout(ctx, pkg, dstDir, zarf.LoadOptions{Streams: opts.Streams, IsPartial: opts.IsPartial})
	if err != nil {
		return nil, err
	}
	result := fromZarfPackageLayout(pkgLayout)
	result.IsPartial = isPartial
	return result, nil
}

// packageLayoutLoaderAdapter converts internal loader options for a public loader.
type packageLayoutLoaderAdapter struct {
	loader ZarfPackageLayoutLoader
}

// LoadPackageLayout delegates package loading through the public loader contract.
func (a packageLayoutLoaderAdapter) LoadPackageLayout(ctx context.Context, pkg *spec.Package, dstDir string, opts zarf.LoadOptions) (*layout.PackageLayout, bool, error) {
	publicLayout, err := a.loader.LoadPackageLayout(ctx, pkg, dstDir, ZarfPackageLayoutLoadOptions{Streams: opts.Streams, IsPartial: opts.IsPartial})
	if err != nil {
		return nil, false, err
	}
	if publicLayout == nil {
		return nil, false, fmt.Errorf("package layout loader returned a nil layout")
	}
	publicLayout.IsPartial = publicLayout.IsPartial || opts.IsPartial
	if publicLayout.Directory != "" {
		cleanDst, err := filepath.Abs(filepath.Clean(dstDir))
		if err != nil {
			return nil, false, err
		}
		cleanLayout, err := filepath.Abs(filepath.Clean(publicLayout.Directory))
		if err != nil {
			return nil, false, err
		}
		if cleanLayout != cleanDst {
			return nil, false, fmt.Errorf("package layout directory %q must be the supplied staging directory %q", publicLayout.Directory, dstDir)
		}
	}
	internalLayout, err := toZarfPackageLayout(ctx, publicLayout, publicLayout.IsPartial)
	if err != nil {
		return nil, false, err
	}
	return internalLayout, publicLayout.IsPartial, nil
}

func fromZarfPackageLayout(pkgLayout *layout.PackageLayout) *ZarfPackageLayout {
	if pkgLayout == nil {
		return nil
	}
	result := &ZarfPackageLayout{
		Directory: pkgLayout.DirPath(),
		Pkg:       ZarfPackage{Components: make([]ZarfPackageComponent, len(pkgLayout.Pkg.Components))},
	}
	if !pkgLayout.IsPushable() && pkgLayout.Digest() != "" {
		result.registryDigest = pkgLayout.Digest()
	}
	for i, component := range pkgLayout.Pkg.Components {
		result.Pkg.Components[i] = ZarfPackageComponent{
			Name:          component.Name,
			Images:        append([]string(nil), component.Images...),
			ImageArchives: make([]ZarfPackageImageArchive, len(component.ImageArchives)),
			privateID:     component.Name,
		}
		for j, archive := range component.ImageArchives {
			result.Pkg.Components[i].ImageArchives[j] = ZarfPackageImageArchive{
				Path:   archive.Path,
				Images: append([]string(nil), archive.Images...),
			}
		}
	}
	return result
}

func toZarfPackageLayout(ctx context.Context, pkgLayout *ZarfPackageLayout, isPartial bool) (*layout.PackageLayout, error) {
	if pkgLayout == nil {
		return nil, nil
	}
	if pkgLayout.Directory != "" {
		result, err := layout.LoadFromDir(ctx, pkgLayout.Directory, layout.PackageLayoutOptions{
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
	if err := applyPublicPackageLayout(result, pkgLayout); err != nil {
		return nil, err
	}
	return result, nil
}

func applyPublicPackageLayout(dst *layout.PackageLayout, src *ZarfPackageLayout) error {
	if dst == nil || src == nil {
		return nil
	}
	original := make(map[string]v1alpha1.ZarfComponent, len(dst.Pkg.Components))
	for _, component := range dst.Pkg.Components {
		original[component.Name] = component
	}
	names := make(map[string]struct{}, len(src.Pkg.Components))
	used := make(map[string]struct{}, len(src.Pkg.Components))
	components := make([]v1alpha1.ZarfComponent, len(src.Pkg.Components))
	for i, component := range src.Pkg.Components {
		if _, ok := names[component.Name]; ok {
			return fmt.Errorf("component name %q appears more than once", component.Name)
		}
		names[component.Name] = struct{}{}
		private := v1alpha1.ZarfComponent{}
		identity := component.privateID
		if identity == "" {
			identity = component.Name
		}
		if component.privateID != "" {
			var ok bool
			private, ok = original[identity]
			if !ok && len(dst.Pkg.Components) > 0 {
				return fmt.Errorf("component %q cannot be reconciled with its original deployment data", component.Name)
			}
		} else if matched, ok := original[identity]; ok {
			private = matched
		} else if len(dst.Pkg.Components) > 0 {
			return fmt.Errorf("component %q cannot be reconciled after public layout mutation", component.Name)
		}
		if _, ok := used[identity]; ok && len(dst.Pkg.Components) > 0 {
			return fmt.Errorf("component identity %q was used more than once", identity)
		}
		used[identity] = struct{}{}
		private.Name = component.Name
		private.Images = append([]string(nil), component.Images...)
		private.ImageArchives = make([]v1alpha1.ImageArchive, len(component.ImageArchives))
		for j, archive := range component.ImageArchives {
			private.ImageArchives[j] = v1alpha1.ImageArchive{
				Path:   archive.Path,
				Images: append([]string(nil), archive.Images...),
			}
		}
		components[i] = private
	}
	dst.Pkg.Components = components
	if src.registryDigest != "" {
		dst.SetRegistryDigest(src.registryDigest)
	}
	return nil
}
