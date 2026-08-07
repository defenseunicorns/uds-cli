// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package sources contains Zarf packager sources
package sources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler/fetcher"
	"github.com/defenseunicorns/uds-cli/src/pkg/cache"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils/boci"
	"github.com/defenseunicorns/uds-cli/src/types"
	goyaml "github.com/goccy/go-yaml"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/signing"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/file"
)

// RemoteBundle is a package source for remote bundles that implements Zarf's packager.PackageSource
type RemoteBundle struct {
	Pkg                     types.Package
	PkgManifestDigest       digest.Digest
	TmpDir                  string
	VerifyBlobOptions       *signing.VerifyBlobOptions
	Remote                  *oci.OrasRemote
	nsOverrides             NamespaceOverrideMap
	bundleCfg               types.BundleConfig
	SkipSignatureValidation bool
}

// LoadPackage loads a Zarf package from a remote bundle
func (r *RemoteBundle) LoadPackage(ctx context.Context, filter filters.ComponentFilterStrategy) (*layout.PackageLayout, []string, error) {
	// todo: progress bar??
	var err error

	if config.Dev {
		if _, ok := r.bundleCfg.DevDeployOpts.Ref[r.Pkg.Name]; ok {
			// create new oras remote for package
			platform := ocispec.Platform{
				Architecture: config.GetArch(),
				OS:           oci.MultiOS,
			}
			// get remote client
			repoUrl := fmt.Sprintf("%s:%s", r.Pkg.Repository, r.Pkg.Ref)
			remote, _ := fetcher.NewZarfOCIRemote(ctx, repoUrl, platform)
			_, err = remote.PullPackage(ctx, r.TmpDir, config.CommonOptions.OCIConcurrency)
		} else {
			_, err = r.downloadPkgFromRemoteBundle()
		}
	} else {
		_, err = r.downloadPkgFromRemoteBundle()
	}

	if err != nil {
		return nil, nil, err
	}

	var pkg v1alpha1.ZarfPackage
	if err = utils.ReadYAMLStrict(filepath.Join(r.TmpDir, layout.ZarfYAML), &pkg); err != nil {
		return nil, nil, err
	}

	// if in dev mode and package is a zarf init config, return an empty package
	if config.Dev && pkg.Kind == v1alpha1.ZarfInitConfig {
		return nil, nil, nil
	}

	// filter pkg components and determine if its a partial pkg
	filteredComps, isPartialPkg, err := handleFilter(pkg, filter)
	if err != nil {
		return nil, nil, err
	}
	pkg.Components = filteredComps

	layoutOpts := layout.PackageLayoutOptions{
		VerifyBlobOptions:    r.VerifyBlobOptions,
		VerificationStrategy: utils.GetPackageVerificationStrategy(r.SkipSignatureValidation),
		IsPartial:            isPartialPkg,
		Filter:               filter,
	}

	pkgLayout, err := loadPackageFromDir(ctx, r.TmpDir, layoutOpts, r.PkgManifestDigest)
	if err != nil {
		return nil, nil, err
	}

	addNamespaceOverrides(&pkgLayout.Pkg, r.nsOverrides)

	// ensure we're using the correct package name as specified by the bundle
	pkgLayout.Pkg.Metadata.Name = r.Pkg.Name
	return pkgLayout, nil, err
}

// LoadPackageMetadata loads a Zarf package's metadata from a remote bundle
func (r *RemoteBundle) LoadPackageMetadata(ctx context.Context, _ bool, _ bool) (v1alpha1.ZarfPackage, []string, error) {
	root, err := r.Remote.FetchRoot(ctx)
	if err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}
	pkgManifestDesc := root.Locate(r.PkgManifestDigest.Encoded())
	if oci.IsEmptyDescriptor(pkgManifestDesc) {
		return v1alpha1.ZarfPackage{}, nil, fmt.Errorf("zarf package %s with manifest digest %s not found", r.Pkg.Name, r.PkgManifestDigest)
	}

	// look at Zarf pkg manifest, grab zarf.yaml desc and download it
	pkgManifest, err := r.Remote.FetchManifest(ctx, pkgManifestDesc)
	if err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}

	var zarfYAMLDesc ocispec.Descriptor
	for _, layer := range pkgManifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == config.ZarfYAML {
			zarfYAMLDesc = layer
			break
		}
	}
	pkgBytes, err := content.FetchAll(ctx, r.Remote, zarfYAMLDesc)
	if err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}
	if err = os.WriteFile(filepath.Join(r.TmpDir, layout.ZarfYAML), pkgBytes, 0600); err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}

	var pkg v1alpha1.ZarfPackage
	if err = goyaml.Unmarshal(pkgBytes, &pkg); err != nil {
		return v1alpha1.ZarfPackage{}, nil, err
	}

	// grab checksums.txt so we can validate pkg integrity
	for _, layer := range pkgManifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == config.ChecksumsTxt {
			checksumBytes, err := content.FetchAll(ctx, r.Remote, layer)
			if err != nil {
				return v1alpha1.ZarfPackage{}, nil, err
			}
			err = os.WriteFile(filepath.Join(r.TmpDir, layout.Checksums), checksumBytes, 0600)
			if err != nil {
				return v1alpha1.ZarfPackage{}, nil, err
			}
			break
		}
	}

	// grab signature(s) if present (key-based: zarf.yaml.sig, keyless: zarf.bundle.sig)
	for _, layer := range pkgManifest.Layers {
		switch layer.Annotations[ocispec.AnnotationTitle] {
		case config.LegacySignature:
			signatureBytes, err := content.FetchAll(ctx, r.Remote, layer)
			if err != nil {
				return v1alpha1.ZarfPackage{}, nil, err
			}
			if err = os.WriteFile(filepath.Join(r.TmpDir, config.LegacySignature), signatureBytes, 0600); err != nil {
				return v1alpha1.ZarfPackage{}, nil, err
			}
		case layout.Bundle:
			bundleSigBytes, err := content.FetchAll(ctx, r.Remote, layer)
			if err != nil {
				return v1alpha1.ZarfPackage{}, nil, err
			}
			if err = os.WriteFile(filepath.Join(r.TmpDir, layout.Bundle), bundleSigBytes, 0600); err != nil {
				return v1alpha1.ZarfPackage{}, nil, err
			}
		}
	}

	// ensure we're using the correct package name as specified by the bundle
	pkg.Metadata.Name = r.Pkg.Name
	return pkg, nil, err
}

// downloadPkgFromRemoteBundle downloads a Zarf package from a remote bundle
func (r *RemoteBundle) downloadPkgFromRemoteBundle() ([]ocispec.Descriptor, error) {
	ctx := context.TODO()
	rootManifest, err := r.Remote.FetchRoot(ctx)
	if err != nil {
		return nil, err
	}

	pkgLayers, err := boci.SelectBundledPackageContent(ctx, rootManifest, r.Remote, r.PkgManifestDigest, r.Pkg.OptionalComponents)
	if err != nil {
		return nil, fmt.Errorf("selecting layers for package %q: %w", r.Pkg.Name, err)
	}

	estimatedBytes := int64(0)
	layersToPull := make([]ocispec.Descriptor, 0, len(pkgLayers))
	for _, layer := range pkgLayers {
		digest := layer.Digest.Encoded()
		// Selected descriptors come from the package manifest and retain the path annotations
		// needed by the file store to reconstruct the Zarf package layout.
		if strings.Contains(layer.Annotations[ocispec.AnnotationTitle], config.BlobsDir) && cache.Exists(digest) {
			dst := filepath.Join(r.TmpDir, "images", config.BlobsDir)
			if err := cache.Use(digest, dst); err != nil {
				return nil, err
			}
		} else {
			layersToPull = append(layersToPull, layer)
			estimatedBytes += layer.Size
		}
	}

	// create local file target for pkg layers
	target, err := file.New(r.TmpDir)
	if err != nil {
		return nil, err
	}
	defer target.Close()
	_, err = boci.CopyLayers(layersToPull, estimatedBytes, r.TmpDir, r.Remote.Repo(), target, r.Pkg.Name)
	if err != nil {
		return nil, err
	}
	if err := boci.MaterializePackagePaths(ctx, r.Remote, r.TmpDir, pkgLayers); err != nil {
		return nil, err
	}

	return pkgLayers, nil
}
