// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/pkg/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/archive"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarftypes "github.com/zarf-dev/zarf/src/types"
)

func loadPackageSource(ctx context.Context, tmpRoot string, opts Options, loadOpts layout.PackageLayoutOptions) (*layout.PackageLayout, error) {
	if isRemoteSource(opts.Source) {
		return pullRemotePackage(ctx, tmpRoot, opts, loadOpts)
	}
	return loadLocalPackage(ctx, tmpRoot, opts.Source, loadOpts)
}

func loadLocalPackage(ctx context.Context, tmpRoot, source string, loadOpts layout.PackageLayoutOptions) (*layout.PackageLayout, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", source, err)
	}
	stageDir := filepath.Join(tmpRoot, "zarf-pkg")
	if info.IsDir() {
		if err := rejectSymlinks(source); err != nil {
			return nil, err
		}
		if err := helpers.CreatePathAndCopy(source, stageDir); err != nil {
			return nil, fmt.Errorf("copying local package: %w", err)
		}
	} else {
		if !isPackageArchive(source) {
			return nil, fmt.Errorf("unsupported local package source %q: expected a Zarf package directory, .tar, or .tar.zst archive", source)
		}
		if err := os.MkdirAll(stageDir, helpers.ReadWriteExecuteUser); err != nil {
			return nil, fmt.Errorf("creating package workspace: %w", err)
		}
		if err := archive.Decompress(ctx, source, stageDir, archive.DecompressOpts{}); err != nil {
			return nil, fmt.Errorf("extracting local package archive %q: %w", source, err)
		}
	}
	pkgLayout, err := layout.LoadFromDir(ctx, stageDir, loadOpts)
	if err != nil {
		return nil, fmt.Errorf("loading local package %q: %w", source, err)
	}
	return pkgLayout, nil
}

func pullRemotePackage(ctx context.Context, tmpRoot string, opts Options, loadOpts layout.PackageLayoutOptions) (*layout.PackageLayout, error) {
	ref := strings.TrimPrefix(opts.Source, "oci://")
	platform := ocispec.Platform{Architecture: opts.Architecture, OS: oci.MultiOS}
	remoteOpts := zoci.RemoteClientOptions{
		RemoteOptions: zarftypes.RemoteOptions{
			PlainHTTP:             opts.PlainHTTP,
			InsecureSkipTLSVerify: opts.SkipTLSVerify,
		},
	}
	remote, err := zoci.NewRemoteWithOptions(ctx, ref, platform, remoteOpts)
	if err != nil {
		return nil, fmt.Errorf("creating OCI remote for %q: %w", ref, err)
	}
	rootDesc, err := remote.ResolveRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving root manifest for %q: %w", ref, err)
	}
	pinnedRef := fmt.Sprintf("%s/%s@%s", remote.Repo().Reference.Registry, remote.Repo().Reference.Repository, rootDesc.Digest)
	remote, err = zoci.NewRemoteWithOptions(ctx, pinnedRef, platform, remoteOpts)
	if err != nil {
		return nil, fmt.Errorf("creating pinned OCI remote for %q: %w", pinnedRef, err)
	}
	root, err := remote.FetchManifest(ctx, rootDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching root manifest for %q: %w", pinnedRef, err)
	}
	pkg, err := zoci.FetchZarfYAML(ctx, root, remote)
	if err != nil {
		return nil, fmt.Errorf("fetching zarf.yaml for %q: %w", pinnedRef, err)
	}
	layers, err := zoci.AssembleLayers(ctx, root, remote, pkg.Components)
	if err != nil {
		return nil, fmt.Errorf("resolving package layers for %q: %w", pinnedRef, err)
	}
	stageDir := filepath.Join(tmpRoot, "zarf-pkg")
	if _, err := remote.PullPackage(ctx, stageDir, opts.Concurrency, layers...); err != nil {
		return nil, fmt.Errorf("pulling package %q: %w", pinnedRef, err)
	}
	pkgLayout, err := layout.LoadFromDir(ctx, stageDir, loadOpts)
	if err != nil {
		return nil, fmt.Errorf("loading package layout for %q: %w", pinnedRef, err)
	}
	return pkgLayout, nil
}

func isRemoteSource(source string) bool {
	if strings.HasPrefix(source, "oci://") {
		return true
	}
	if _, err := os.Stat(source); err == nil || !errors.Is(err, os.ErrNotExist) {
		return false
	}
	if strings.Contains(source, "://") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") || strings.Contains(source, `\`) || strings.Contains(source, " ") {
		return false
	}
	if strings.HasPrefix(source, "localhost/") || strings.HasPrefix(source, "localhost:") {
		return strings.Contains(source, "/")
	}
	if strings.HasSuffix(source, ".hcl") || strings.Contains(source, ".tar") || strings.HasSuffix(source, ".yaml") || strings.HasSuffix(source, ".yml") {
		return false
	}
	return strings.Contains(source, ".") && strings.Contains(source, "/")
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
			return fmt.Errorf("local package %q contains unsupported symlink %q", root, path)
		}
		return nil
	})
}
