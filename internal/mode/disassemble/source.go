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
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarftypes "github.com/zarf-dev/zarf/src/types"
)

func loadPackageSource(ctx context.Context, tmpRoot string, opts Options, loadOpts layout.PackageLayoutOptions) (*layout.PackageLayout, error) {
	info, err := os.Stat(opts.Source)
	switch {
	case err == nil && info.IsDir():
		return loadLocalPackageDir(ctx, tmpRoot, opts.Source, loadOpts)
	case err == nil && !isPackageArchive(opts.Source):
		return nil, fmt.Errorf("unsupported local package source %q: expected a Zarf package directory, .tar, or .tar.zst archive", opts.Source)
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("stat %q: %w", opts.Source, err)
	case err != nil && !looksLikeRemoteSource(opts.Source):
		return nil, fmt.Errorf("stat %q: %w", opts.Source, err)
	}

	source := opts.Source
	if looksLikeRemoteSource(source) && !strings.HasPrefix(source, "oci://") {
		source = "oci://" + source
	}
	pkgLayout, err := packager.LoadPackage(ctx, source, packager.LoadOptions{
		Architecture:         opts.Architecture,
		Filter:               loadOpts.Filter,
		OCIConcurrency:       opts.Concurrency,
		VerificationStrategy: loadOpts.VerificationStrategy,
		RemoteOptions: zarftypes.RemoteOptions{
			PlainHTTP:             opts.PlainHTTP,
			InsecureSkipTLSVerify: opts.SkipTLSVerify,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("loading package %q: %w", opts.Source, err)
	}
	return pkgLayout, nil
}

func loadLocalPackageDir(ctx context.Context, tmpRoot, source string, loadOpts layout.PackageLayoutOptions) (*layout.PackageLayout, error) {
	stageDir := filepath.Join(tmpRoot, "zarf-pkg")
	if err := rejectSymlinks(source); err != nil {
		return nil, err
	}
	if err := helpers.CreatePathAndCopy(source, stageDir); err != nil {
		return nil, fmt.Errorf("copying local package: %w", err)
	}
	pkgLayout, err := layout.LoadFromDir(ctx, stageDir, loadOpts)
	if err != nil {
		return nil, fmt.Errorf("loading local package %q: %w", source, err)
	}
	return pkgLayout, nil
}

func looksLikeRemoteSource(source string) bool {
	if strings.HasPrefix(source, "oci://") {
		return true
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
