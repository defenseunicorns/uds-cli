// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

func copyPackageLevelAssets(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir string, pkg *v1alpha1.ZarfPackage) error {
	if err := localizeOptionalPackageAsset(pkgLayout.DirPath(), outputDir, layout.ValuesYAML, []string{layout.ValuesYAML}, func(files []string) {
		pkg.Values.Files = files
	}); err != nil {
		return err
	}

	if err := localizeOptionalPackageAsset(pkgLayout.DirPath(), outputDir, layout.ValuesSchema, layout.ValuesSchema, func(schema string) {
		pkg.Values.Schema = schema
	}); err != nil {
		return err
	}

	if len(pkg.Documentation) == 0 {
		return nil
	}
	docDir := filepath.Join(outputDir, "documentation")
	if err := os.MkdirAll(docDir, helpers.ReadWriteExecuteUser); err != nil {
		return fmt.Errorf("creating documentation directory: %w", err)
	}
	if err := pkgLayout.GetDocumentation(ctx, docDir, nil); err != nil {
		return fmt.Errorf("extracting documentation: %w", err)
	}

	localized := make(map[string]string, len(pkg.Documentation))
	for key, name := range layout.GetDocumentationFileNames(pkg.Documentation) {
		localized[key] = filepath.ToSlash(filepath.Join("documentation", name))
	}
	pkg.Documentation = localized
	return nil
}

func localizeOptionalPackageAsset[T any](packageDir, outputDir, name string, localized T, update func(T)) error {
	source := filepath.Join(packageDir, name)
	if _, err := os.Stat(source); err != nil {
		if os.IsNotExist(err) {
			var zero T
			update(zero)
			return nil
		}
		return fmt.Errorf("checking %s: %w", name, err)
	}
	if err := helpers.CreatePathAndCopy(source, filepath.Join(outputDir, name)); err != nil {
		return fmt.Errorf("copying %s: %w", name, err)
	}
	update(localized)
	return nil
}
