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
	hasValues, err := copyOptionalPackageAsset(pkgLayout.DirPath(), outputDir, layout.ValuesYAML)
	if err != nil {
		return err
	}
	if hasValues {
		pkg.Values.Files = []string{layout.ValuesYAML}
	} else {
		pkg.Values.Files = nil
	}

	hasSchema, err := copyOptionalPackageAsset(pkgLayout.DirPath(), outputDir, layout.ValuesSchema)
	if err != nil {
		return err
	}
	if hasSchema {
		pkg.Values.Schema = layout.ValuesSchema
	} else {
		pkg.Values.Schema = ""
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

func copyOptionalPackageAsset(packageDir, outputDir, name string) (bool, error) {
	source := filepath.Join(packageDir, name)
	if _, err := os.Stat(source); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking %s: %w", name, err)
	}
	if err := helpers.CreatePathAndCopy(source, filepath.Join(outputDir, name)); err != nil {
		return false, fmt.Errorf("copying %s: %w", name, err)
	}
	return true, nil
}
