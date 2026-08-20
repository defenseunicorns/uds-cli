// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"fmt"
	"os"

	"github.com/defenseunicorns/pkg/oci"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
)

var _ PackageSource = &remoteSource{}

func (s *remoteSource) newZociRemote(ctx context.Context) (*zoci.Remote, error) {
	return s.newZociRemoteForRef(ctx, s.ref)
}

func (s *remoteSource) newZociRemoteForRef(ctx context.Context, ref string) (*zoci.Remote, error) {
	platform := ocispec.Platform{Architecture: s.arch, OS: oci.MultiOS}
	plainHTTP, err := udsoci.ResolvePlainHTTP(ctx, ref, bundleinternal.ConfigOptions{PlainHTTP: s.opts.PlainHTTP, SkipTLSVerify: s.opts.SkipTLSVerify}, nil)
	if err != nil {
		return nil, err
	}
	return zoci.NewRemote(ctx, ref, platform,
		oci.WithPlainHTTP(plainHTTP),
		oci.WithInsecureSkipVerify(s.opts.SkipTLSVerify),
	)
}

func (s *remoteSource) resolveFilteredLayers(ctx context.Context, filter filters.ComponentFilterStrategy) (*resolvedLayers, error) {
	remote, err := s.newZociRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating OCI remote for %q: %w: %w", s.ref, ErrCreateOCIRemote, err)
	}
	rootDesc, err := remote.ResolveRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving root manifest for %q: %w: %w", s.ref, ErrResolveRootManifest, err)
	}
	pinnedRef := pinnedRemoteReference(remote, rootDesc)
	remote, err = s.newZociRemoteForRef(ctx, pinnedRef)
	if err != nil {
		return nil, fmt.Errorf("creating pinned OCI remote for %q: %w: %w", pinnedRef, ErrCreateOCIRemote, err)
	}
	root, err := remote.FetchManifest(ctx, rootDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching root manifest for %q: %w: %w", pinnedRef, ErrFetchRootManifest, err)
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
	return &resolvedLayers{remote: remote, root: root, layers: layers, isPartial: isPartial}, nil
}

func pinnedRemoteReference(remote *zoci.Remote, desc ocispec.Descriptor) string {
	ref := remote.Repo().Reference
	return fmt.Sprintf("%s/%s@%s", ref.Registry, ref.Repository, desc.Digest)
}

func (s *remoteSource) concurrency() int {
	if s.opts.Concurrency > 0 {
		return s.opts.Concurrency
	}
	return zoci.DefaultConcurrency
}

// PullFiltered pulls only the Zarf layers selected for the requested components.
func (s *remoteSource) PullFiltered(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions) (*layout.PackageLayout, error) {
	pkgLayout, _, err := s.pullFilteredWithSelection(ctx, tmpDir, loadOptions)
	return pkgLayout, err
}

func (s *remoteSource) pullFilteredWithSelection(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions) (*layout.PackageLayout, []ocispec.Descriptor, error) {
	resolved, err := s.resolveFilteredLayers(ctx, loadOptions.Filter)
	if err != nil {
		return nil, nil, err
	}
	if _, err := resolved.remote.PullPackage(ctx, tmpDir, s.concurrency(), resolved.layers...); err != nil {
		return nil, nil, fmt.Errorf("pulling package %q: %w: %w", s.ref, ErrPullPackage, err)
	}
	loadOptions.IsPartial = resolved.isPartial
	pkgLayout, err := layout.LoadFromDir(ctx, tmpDir, loadOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("loading package layout for %q: %w: %w", s.ref, ErrLoadPackage, err)
	}
	return pkgLayout, resolved.layers, nil
}

// VerifyAndIngestFiltered verifies one downloaded layout and copies those exact
// bytes into the bundle's ORAS content store.
func (s *remoteSource) VerifyAndIngestFiltered(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions, store *udsoci.Store) ([]ocispec.Descriptor, error) {
	pkgLayout, selectedLayers, err := s.pullFilteredWithSelection(ctx, tmpDir, loadOptions)
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

// IngestFiltered downloads the selected package once and copies its canonical
// Zarf v0.83 OCI representation into the bundle store.
func (s *remoteSource) IngestFiltered(ctx context.Context, filter filters.ComponentFilterStrategy, store *udsoci.Store) ([]ocispec.Descriptor, error) {
	tmpDir, err := os.MkdirTemp(s.opts.TmpDir, "uds-package-ingest-*")
	if err != nil {
		return nil, fmt.Errorf("creating package ingest workspace: %w: %w", ErrCreatePackageWorkspace, err)
	}
	pkgLayout, selectedLayers, err := s.pullFilteredWithSelection(ctx, tmpDir, layout.PackageLayoutOptions{
		Filter:               filter,
		VerificationStrategy: layout.VerifyNever,
	})
	if err != nil {
		_ = os.RemoveAll(tmpDir)
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
