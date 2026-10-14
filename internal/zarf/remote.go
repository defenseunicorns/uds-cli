// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/oci"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
	"oras.land/oras-go/v2/registry"
)

var _ PackageSource = &remoteSource{}

// LoadPackageSpec resolves only the OCI root manifest and filtered zarf.yaml.
func (s *remoteSource) LoadPackageSpec(ctx context.Context, filter filters.ComponentFilterStrategy) (*PackageSpec, error) {
	var spec *PackageSpec
	err := s.withCacheFallback(ctx, func(source *remoteSource) error {
		var err error
		spec, err = source.loadPackageSpec(ctx, filter)
		return err
	})
	return spec, err
}

func (s *remoteSource) loadPackageSpec(ctx context.Context, filter filters.ComponentFilterStrategy) (*PackageSpec, error) {
	remote, err := s.newZociRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating OCI remote for %q: %w: %w", s.ref, ErrCreateOCIRemote, err)
	}
	rootDesc, err := remote.ResolveRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving root manifest for %q: %w: %w", s.ref, ErrResolveRootManifest, err)
	}
	s.resolvedRoot = &rootDesc
	root, err := remote.FetchManifest(ctx, rootDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching root manifest for %q: %w: %w", s.ref, ErrFetchRootManifest, err)
	}
	definition, _, err := filteredPackageDefinition(ctx, root, remote, filter)
	if err != nil {
		return nil, fmt.Errorf("loading package specification for %q: %w", s.ref, err)
	}
	return packageSpecFromDefinition(definition, rootDesc.Digest.String()), nil
}

func (s *remoteSource) newZociRemote(ctx context.Context) (*zoci.Remote, error) {
	ref, err := s.resolvedReference()
	if err != nil {
		return nil, err
	}
	return s.newZociRemoteForRef(ctx, ref)
}

func (s *remoteSource) resolvedReference() (string, error) {
	if s.resolvedRoot == nil {
		return s.ref, nil
	}
	ref, err := registry.ParseReference(s.ref)
	if err != nil {
		return "", err
	}
	ref.Reference = s.resolvedRoot.Digest.String()
	return ref.String(), nil
}

func (s *remoteSource) newZociRemoteForRef(ctx context.Context, ref string) (*zoci.Remote, error) {
	platform := ocispec.Platform{Architecture: s.arch, OS: oci.MultiOS}
	plainHTTP, err := udsoci.ResolvePlainHTTP(ctx, ref, bundleinternal.ConfigOptions{PlainHTTP: s.opts.PlainHTTP, SkipTLSVerify: s.opts.SkipTLSVerify}, nil)
	if err != nil {
		return nil, err
	}
	// Zarf appends images/ to CachePath, so this client's OCI cache lives at
	// <uds-cache>/bundle-oci/images/ instead of <uds-cache>/images/.
	cachePath := udsoci.BundleCachePath(s.opts.CacheDir)
	cacheStorePath := filepath.Join(cachePath, zoci.ImageCacheDirectory)
	if cachePath != "" {
		s.streams.Debug("initializing package OCI cache", "path", cacheStorePath)
	} else {
		s.streams.Debug("package OCI caching disabled", "reason", "no cache directory configured")
	}
	remote, err := zoci.NewRemoteWithOptions(ctx, ref, platform, zoci.RemoteClientOptions{
		CachePath: cachePath,
		RemoteOptions: zarfTypes.RemoteOptions{
			PlainHTTP:             plainHTTP,
			InsecureSkipTLSVerify: s.opts.SkipTLSVerify,
		},
	})
	if err != nil {
		return nil, err
	}
	if cachePath != "" {
		// Upstream opens or creates the store; this does not claim that this
		// process created it or that any content has been reused yet.
		s.streams.Debug("package OCI cache initialized", "path", cacheStorePath)
	}
	return remote, nil
}

func (s *remoteSource) resolveFilteredLayers(ctx context.Context, filter filters.ComponentFilterStrategy) (*resolvedLayers, error) {
	remote, err := s.newZociRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating OCI remote for %q: %w: %w", s.ref, ErrCreateOCIRemote, err)
	}
	var rootDesc ocispec.Descriptor
	if s.resolvedRoot != nil {
		rootDesc = *s.resolvedRoot
	} else {
		rootDesc, err = remote.ResolveRoot(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolving root manifest for %q: %w: %w", s.ref, ErrResolveRootManifest, err)
		}
	}
	s.resolvedRoot = &rootDesc
	remote, err = s.newZociRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating pinned OCI remote for %q: %w: %w", s.ref, ErrCreateOCIRemote, err)
	}
	root, err := remote.FetchManifest(ctx, rootDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching root manifest for %q: %w: %w", s.ref, ErrFetchRootManifest, err)
	}

	layers := root.Layers
	isPartial := false
	if !oci.IsEmptyDescriptor(root.Locate(layout.ZarfYAML)) {
		layers, isPartial, err = selectZarfLayers(ctx, root, remote, filter)
		if err != nil {
			return nil, fmt.Errorf("resolving layers for %q: %w: %w", s.ref, ErrResolvePackageLayers, err)
		}
	}
	s.streams.Debug("resolved package layers", "ref", s.ref, "layers", len(layers), "partial", isPartial)
	return &resolvedLayers{remote: remote, root: root, rootDesc: rootDesc, layers: layers, isPartial: isPartial}, nil
}

func (s *remoteSource) concurrency() int {
	if s.opts.Concurrency > 0 {
		return s.opts.Concurrency
	}
	return zoci.DefaultConcurrency
}

// PullFiltered pulls only the Zarf layers selected for the requested components.
func (s *remoteSource) PullFiltered(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions) (*layout.PackageLayout, error) {
	pkgLayout, _, digest, err := s.pullFilteredWithSelection(ctx, tmpDir, loadOptions)
	if err == nil {
		pkgLayout.SetRegistryDigest(digest)
	}
	return pkgLayout, err
}

func (s *remoteSource) pullFilteredWithSelection(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions) (*layout.PackageLayout, []ocispec.Descriptor, string, error) {
	var pkgLayout *layout.PackageLayout
	var selected []ocispec.Descriptor
	var digest string
	err := s.withCacheFallback(ctx, func(source *remoteSource) error {
		var err error
		pkgLayout, selected, digest, err = source.pullFilteredAttempt(ctx, tmpDir, loadOptions)
		return err
	})
	return pkgLayout, selected, digest, err
}

// withCacheFallback retries retrieval once, never package deployment or bundle
// publication. A copy disables caching without changing the caller's options.
func (s *remoteSource) withCacheFallback(ctx context.Context, attempt func(*remoteSource) error) error {
	err := attempt(s)
	if ctx.Err() != nil || errors.Is(err, ErrLoadPackage) || errors.Is(err, ErrCreatePackageWorkspace) || !udsoci.IsCacheStorageError(err, s.opts.CacheDir) {
		return err
	}
	s.streams.Warn("package cache unavailable; retrying retrieval once without caching", "ref", s.ref, "cache", udsoci.BundleCachePath(s.opts.CacheDir))
	retry := *s
	retry.opts.CacheDir = ""
	// Preserve any root descriptor resolved before the cache failure so a
	// moving tag cannot switch packages between attempts.
	if retryErr := attempt(&retry); retryErr != nil {
		return errors.Join(err, fmt.Errorf("retrying package retrieval without caching: %w", retryErr))
	}
	s.resolvedRoot = retry.resolvedRoot
	return nil
}

func (s *remoteSource) pullFilteredAttempt(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions) (*layout.PackageLayout, []ocispec.Descriptor, string, error) {
	resolved, err := s.resolveFilteredLayers(ctx, loadOptions.Filter)
	if err != nil {
		return nil, nil, "", err
	}
	// Own only this child directory. On failure, discard partial downloads
	// before retrying; never erase files from the caller's staging directory.
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return nil, nil, "", fmt.Errorf("creating package staging directory: %w: %w", ErrCreatePackageWorkspace, err)
	}
	stageDir, err := os.MkdirTemp(tmpDir, "uds-package-download-*")
	if err != nil {
		return nil, nil, "", fmt.Errorf("creating package download workspace: %w: %w", ErrCreatePackageWorkspace, err)
	}
	keep := false
	defer func() {
		if !keep {
			if err := os.RemoveAll(stageDir); err != nil {
				s.streams.Warn("failed to remove package download workspace", "path", stageDir, "error", err)
			}
		}
	}()
	if _, err := resolved.remote.PullPackage(ctx, stageDir, s.concurrency(), resolved.layers...); err != nil {
		return nil, nil, "", fmt.Errorf("pulling package %q: %w: %w", s.ref, ErrPullPackage, err)
	}
	loadOptions.IsPartial = resolved.isPartial
	pkgLayout, err := layout.LoadFromDir(ctx, stageDir, loadOptions)
	if err != nil {
		return nil, nil, "", fmt.Errorf("loading package layout for %q: %w: %w", s.ref, ErrLoadPackage, err)
	}
	keep = true
	return pkgLayout, resolved.layers, resolved.rootDesc.Digest.String(), nil
}

// VerifyAndIngestFiltered verifies one downloaded layout and copies those exact
// bytes into the bundle's ORAS content store.
func (s *remoteSource) VerifyAndIngestFiltered(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions, store *udsoci.Store) ([]ocispec.Descriptor, error) {
	pkgLayout, selectedLayers, _, err := s.pullFilteredWithSelection(ctx, tmpDir, loadOptions)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := pkgLayout.Cleanup(); err != nil {
			s.streams.Warn("failed to remove verified package layout", "path", pkgLayout.DirPath(), "error", err)
		}
	}()
	desc, err := copySelectedPackage(ctx, pkgLayout, selectedLayers, store)
	if err != nil {
		return nil, fmt.Errorf("ingesting verified package %q: %w: %w", s.ref, ErrIngestPackage, err)
	}
	return []ocispec.Descriptor{desc}, nil
}

// IngestFiltered retrieves the selected package and copies its canonical
// Zarf v0.83 OCI representation into the bundle store.
func (s *remoteSource) IngestFiltered(ctx context.Context, filter filters.ComponentFilterStrategy, store *udsoci.Store) ([]ocispec.Descriptor, error) {
	tmpDir, err := os.MkdirTemp(s.opts.TmpDir, "uds-package-ingest-*")
	if err != nil {
		return nil, fmt.Errorf("creating package ingest workspace: %w: %w", ErrCreatePackageWorkspace, err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			s.streams.Warn("failed to remove package ingest workspace", "path", tmpDir, "error", err)
		}
	}()
	pkgLayout, selectedLayers, _, err := s.pullFilteredWithSelection(ctx, tmpDir, layout.PackageLayoutOptions{
		Filter:               filter,
		VerificationStrategy: layout.VerifyNever,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := pkgLayout.Cleanup(); err != nil {
			s.streams.Warn("failed to remove package layout", "path", pkgLayout.DirPath(), "error", err)
		}
	}()
	desc, err := copySelectedPackage(ctx, pkgLayout, selectedLayers, store)
	if err != nil {
		return nil, fmt.Errorf("ingesting package %q: %w: %w", s.ref, ErrIngestPackage, err)
	}
	return []ocispec.Descriptor{desc}, nil
}
