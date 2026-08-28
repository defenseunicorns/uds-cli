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
	valuesPath := filepath.Join(pkgLayout.DirPath(), layout.ValuesYAML)
	if _, err := os.Stat(valuesPath); err == nil {
		if err := helpers.CreatePathAndCopy(valuesPath, filepath.Join(outputDir, layout.ValuesYAML)); err != nil {
			return fmt.Errorf("copying %s: %w", layout.ValuesYAML, err)
		}
		pkg.Values.Files = []string{layout.ValuesYAML}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", layout.ValuesYAML, err)
	} else {
		pkg.Values.Files = nil
	}

	schemaPath := filepath.Join(pkgLayout.DirPath(), layout.ValuesSchema)
	if _, err := os.Stat(schemaPath); err == nil {
		if err := helpers.CreatePathAndCopy(schemaPath, filepath.Join(outputDir, layout.ValuesSchema)); err != nil {
			return fmt.Errorf("copying %s: %w", layout.ValuesSchema, err)
		}
		pkg.Values.Schema = layout.ValuesSchema
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", layout.ValuesSchema, err)
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
