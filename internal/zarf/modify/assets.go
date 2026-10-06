// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package modify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/helpers/v2"
	internalzarf "github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/mholt/archives"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/pkg/archive"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

func localizePackageLevelAssets(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, tmpRoot string, pkg *api.Package) error {
	packageRoot, err := os.OpenRoot(pkgLayout.DirPath())
	if err != nil {
		return fmt.Errorf("opening package assets: %w", err)
	}
	defer func() { _ = packageRoot.Close() }()
	outputRoot, err := os.OpenRoot(outputDir)
	if err != nil {
		return fmt.Errorf("opening output directory: %w", err)
	}
	defer func() { _ = outputRoot.Close() }()

	if err := localizeOptionalPackageAsset(ctx, packageRoot, outputRoot, layout.ValuesYAML, []string{layout.ValuesYAML}, func(files []string) {
		pkg.Values.Files = files
	}); err != nil {
		return err
	}

	if err := localizeOptionalPackageAsset(ctx, packageRoot, outputRoot, layout.ValuesSchema, layout.ValuesSchema, func(schema string) {
		pkg.Values.Schema = schema
	}); err != nil {
		return err
	}

	if len(pkg.Documentation) == 0 {
		return nil
	}
	docArchive, err := packageRoot.Open(layout.DocumentationTar)
	if err != nil {
		return fmt.Errorf("opening documentation archive: %w", err)
	}
	defer func() { _ = docArchive.Close() }()
	docDir := filepath.Join(tmpRoot, "documentation")
	if err := archive.DecompressStream(ctx, docArchive, docDir, archive.DecompressOpts{Extractor: archives.Tar{}}); err != nil {
		return fmt.Errorf("extracting documentation: %w", err)
	}
	docRoot, err := os.OpenRoot(docDir)
	if err != nil {
		return fmt.Errorf("opening documentation directory: %w", err)
	}
	defer func() { _ = docRoot.Close() }()

	localized := make(map[string]string, len(pkg.Documentation))
	for key, name := range layout.GetDocumentationFileNames(pkg.Documentation) {
		if !filepath.IsLocal(name) {
			return fmt.Errorf("documentation %q has invalid filename %q", key, name)
		}
		rel := filepath.Join("documentation", name)
		if err := outputRoot.MkdirAll(filepath.Dir(rel), helpers.ReadWriteExecuteUser); err != nil {
			return fmt.Errorf("creating documentation directory: %w", err)
		}
		if err := internalzarf.CopyFileContentsBetweenRoots(ctx, docRoot, name, outputRoot, rel); err != nil {
			return fmt.Errorf("copying documentation %q: %w", key, err)
		}
		localized[key] = filepath.ToSlash(rel)
	}
	pkg.Documentation = localized
	return nil
}

func localizeOptionalPackageAsset[T any](ctx context.Context, packageRoot, outputRoot *os.Root, name string, localized T, update func(T)) error {
	if _, err := packageRoot.Stat(name); err != nil {
		if os.IsNotExist(err) {
			var zero T
			update(zero)
			return nil
		}
		return fmt.Errorf("checking %s: %w", name, err)
	}
	if err := internalzarf.CopyFileContentsBetweenRoots(ctx, packageRoot, name, outputRoot, name); err != nil {
		return fmt.Errorf("copying %s: %w", name, err)
	}
	update(localized)
	return nil
}
