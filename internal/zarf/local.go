// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/archive"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

var _ PackageSource = &localSource{}

func (s *localSource) resolvedPath() string {
	if filepath.IsAbs(s.path) {
		return s.path
	}
	return filepath.Join(s.bundleDir, s.path)
}

// PullFiltered loads a Zarf directory or archive through Zarf's canonical
// PackageLayout APIs. Arbitrary OCI layout directories are intentionally unsupported.
func (s *localSource) PullFiltered(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions) (*layout.PackageLayout, error) {
	if strings.TrimSpace(s.path) == "" {
		return nil, ErrLocalSourcePathRequired
	}
	path := s.resolvedPath()
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w: %w", path, ErrStatLocalPackage, err)
	}
	if !info.IsDir() {
		if !isPackageArchive(path) {
			return nil, fmt.Errorf("unsupported local package source %q: %w", s.path, ErrInvalidLocalPackageSource)
		}
		pkgLayout, err := loadPackageArchive(ctx, path, tmpDir, loadOptions)
		if err != nil {
			return nil, fmt.Errorf("loading local package archive %q: %w: %w", s.path, ErrLoadPackage, err)
		}
		return pkgLayout, nil
	}
	if !isZarfPackage(path) {
		return nil, fmt.Errorf("unsupported local package source %q: not a Zarf package directory or package archive: %w", path, ErrInvalidLocalPackageSource)
	}
	if err := rejectSymlinks(path); err != nil {
		return nil, err
	}

	stageDir := filepath.Join(tmpDir, "zarf-pkg")
	if err := helpers.CreatePathAndCopy(path, stageDir); err != nil {
		return nil, fmt.Errorf("copying local package to temp dir: %w: %w", ErrCopyLocalPackage, err)
	}
	pkgLayout, err := layout.LoadFromDir(ctx, stageDir, loadOptions)
	if err != nil {
		return nil, fmt.Errorf("loading local package directory %q: %w: %w", s.path, ErrLoadPackage, err)
	}
	return pkgLayout, nil
}

// VerifyAndIngestFiltered verifies a private staged copy and ingests those
// exact bytes through the PackageLayout ORAS target.
func (s *localSource) VerifyAndIngestFiltered(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions, store *udsoci.Store) ([]ocispec.Descriptor, error) {
	pkgLayout, err := s.PullFiltered(ctx, tmpDir, loadOptions)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := pkgLayout.Cleanup(); err != nil {
			s.streams.Warn("failed to remove verified package layout", "path", pkgLayout.DirPath(), "error", err)
		}
	}()
	return s.ingestPackageLayout(ctx, pkgLayout, loadOptions.Filter, store)
}

// IngestFiltered loads a local package with Zarf and copies its canonical OCI
// representation into the bundle store.
func (s *localSource) IngestFiltered(ctx context.Context, filter filters.ComponentFilterStrategy, store *udsoci.Store) ([]ocispec.Descriptor, error) {
	if strings.TrimSpace(s.path) == "" {
		return nil, ErrLocalSourcePathRequired
	}
	path := s.resolvedPath()
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w: %w", path, ErrStatLocalPackage, err)
	}
	loadOptions := layout.PackageLayoutOptions{Filter: filter, VerificationStrategy: layout.VerifyNever}

	var pkgLayout *layout.PackageLayout
	switch {
	case info.IsDir() && isZarfPackage(path):
		if err := rejectSymlinks(path); err != nil {
			return nil, err
		}
		pkgLayout, err = layout.LoadFromDir(ctx, path, loadOptions)
	case !info.IsDir() && isPackageArchive(path):
		pkgLayout, err = loadPackageArchive(ctx, path, s.tmpDir, loadOptions)
	default:
		return nil, fmt.Errorf("unsupported local package source %q: not a Zarf package directory or package archive: %w", path, ErrInvalidLocalPackageSource)
	}
	if err != nil {
		return nil, fmt.Errorf("loading local package %q: %w: %w", s.path, ErrLoadPackage, err)
	}
	if !info.IsDir() {
		defer func() {
			if err := pkgLayout.Cleanup(); err != nil {
				s.streams.Warn("failed to remove package layout", "path", pkgLayout.DirPath(), "error", err)
			}
		}()
	}
	return s.ingestPackageLayout(ctx, pkgLayout, filter, store)
}

func isPackageArchive(path string) bool {
	return strings.HasSuffix(path, ".tar.zst") || strings.HasSuffix(path, ".tar")
}

func rejectSymlinks(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("local package %q contains unsupported symlink %q: %w", root, path, ErrUnsupportedPackageSymlink)
		}
		return nil
	})
}

func loadPackageArchive(ctx context.Context, path, tmpDir string, loadOptions layout.PackageLayoutOptions) (*layout.PackageLayout, error) {
	stageDir, err := os.MkdirTemp(tmpDir, "zarf-pkg-*")
	if err != nil {
		return nil, fmt.Errorf("creating package archive workspace: %w: %w", ErrCreatePackageWorkspace, err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stageDir)
		}
	}()
	if err := archive.Decompress(ctx, path, stageDir, archive.DecompressOpts{}); err != nil {
		return nil, fmt.Errorf("extracting local package archive %q: %w: %w", path, ErrExtractLocalPackage, err)
	}
	pkgLayout, err := layout.LoadFromDir(ctx, stageDir, loadOptions)
	if err != nil {
		return nil, fmt.Errorf("loading local package archive %q: %w: %w", path, ErrLoadPackage, err)
	}
	cleanup = false
	return pkgLayout, nil
}

func (s *localSource) ingestPackageLayout(ctx context.Context, pkgLayout *layout.PackageLayout, filter filters.ComponentFilterStrategy, store *udsoci.Store) ([]ocispec.Descriptor, error) {
	root, err := pkgLayout.Manifest()
	if err != nil {
		return nil, fmt.Errorf("%w from %q: %w", ErrReadCanonicalManifest, s.path, err)
	}
	layers, _, err := selectZarfLayers(ctx, root, pkgLayout, filter)
	if err != nil {
		return nil, fmt.Errorf("selecting layers for %q: %w: %w", s.path, ErrResolvePackageLayers, err)
	}
	desc, err := copySelectedPackage(ctx, pkgLayout, layers, store)
	if err != nil {
		return nil, fmt.Errorf("ingesting package %q: %w: %w", s.path, ErrIngestPackage, err)
	}
	return []ocispec.Descriptor{desc}, nil
}
