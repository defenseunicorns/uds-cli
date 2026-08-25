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
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/api/v1beta1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// ZarfPackageLayout is the supported public subset of
// github.com/zarf-dev/zarf/src/pkg/packager/layout.PackageLayout exposed during
// bundle deploy. Fields not exposed here are preserved by the adapter.
type ZarfPackageLayout struct {
	dirPath           string
	Pkg               ZarfPackage
	digest            string
	packageDefinition api.PackageDefinition
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
	zarfPkg := pkgLayout.AsV1alpha1()
	result := &ZarfPackageLayout{
		dirPath:           pkgLayout.DirPath(),
		Pkg:               ZarfPackage{Components: make([]ZarfPackageComponent, len(zarfPkg.Components))},
		packageDefinition: pkgLayout.PackageDefinition,
	}
	if !pkgLayout.IsPushable() && pkgLayout.Digest() != "" {
		result.digest = pkgLayout.Digest()
	}
	for i, component := range zarfPkg.Components {
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
	if pkgLayout.packageDefinition.OriginalAPIVersion() != "" {
		result.PackageDefinition = pkgLayout.packageDefinition
	}
	if err := applyPublicPackageLayout(result, pkgLayout); err != nil {
		return nil, err
	}
	return result, nil
}

func applyPublicPackageLayout(dst *layout.PackageLayout, src *ZarfPackageLayout) error {
	if dst == nil || src == nil {
		return nil
	}
	zarfPkg := dst.AsV1alpha1()
	original := make(map[string]v1alpha1.ZarfComponent, len(zarfPkg.Components))
	for _, component := range zarfPkg.Components {
		original[component.Name] = component
	}
	names := make(map[string]struct{}, len(src.Pkg.Components))
	used := make(map[string]struct{}, len(src.Pkg.Components))
	components := make([]v1alpha1.ZarfComponent, len(src.Pkg.Components))
	identities := make([]string, len(src.Pkg.Components))
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
			if !ok && len(zarfPkg.Components) > 0 {
				return fmt.Errorf("component %q cannot be reconciled with its original deployment data", component.Name)
			}
		} else if matched, ok := original[identity]; ok {
			private = matched
		} else if len(zarfPkg.Components) > 0 {
			return fmt.Errorf("component %q cannot be reconciled after public layout mutation", component.Name)
		}
		if _, ok := used[identity]; ok && len(zarfPkg.Components) > 0 {
			return fmt.Errorf("component identity %q was used more than once", identity)
		}
		used[identity] = struct{}{}
		identities[i] = identity
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
	zarfPkg.Components = components
	if dst.PackageDefinition.OriginalAPIVersion() == v1beta1.APIVersion {
		betaPkg := dst.AsV1beta1()
		original := make(map[string]v1beta1.Component, len(betaPkg.Components))
		for _, component := range betaPkg.Components {
			original[component.Name] = component
		}
		betaComponents := make([]v1beta1.Component, len(components))
		for i, component := range components {
			private := original[identities[i]]
			private.Name = component.Name
			private.Images = preserveV1beta1Images(private.Images, component.Images)
			private.ImageArchives = make([]v1beta1.ImageArchive, len(component.ImageArchives))
			for j, archive := range component.ImageArchives {
				private.ImageArchives[j] = v1beta1.ImageArchive{
					Path:   archive.Path,
					Images: append([]string(nil), archive.Images...),
				}
			}
			betaComponents[i] = private
		}
		betaPkg.Components = betaComponents
		dst.PackageDefinition = api.NewPackageDefinitionFromV1beta1(betaPkg)
	} else {
		dst.PackageDefinition = api.NewPackageDefinitionFromV1alpha1(zarfPkg)
	}
	if src.digest != "" {
		dst.SetRegistryDigest(src.digest)
	}
	return nil
}

func preserveV1beta1Images(original []v1beta1.Image, names []string) []v1beta1.Image {
	byName := make(map[string]v1beta1.Image, len(original))
	for _, image := range original {
		byName[image.Name] = image
	}
	images := make([]v1beta1.Image, len(names))
	for i, name := range names {
		images[i] = byName[name]
		images[i].Name = name
	}
	return images
}
