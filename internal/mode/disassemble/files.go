// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

func localizeFiles(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, tmpRoot string, component *v1alpha1.ZarfComponent) error {
	filesDir, err := pkgLayout.GetComponentDir(ctx, tmpRoot, component.Name, layout.FilesComponentDir)
	if err != nil {
		return fmt.Errorf("reading file assets for component %s: %w", component.Name, err)
	}
	for idx := range component.Files {
		file := &component.Files[idx]
		src, err := sourceFromIndexedDir(filesDir, idx, file.Target)
		if err != nil {
			return err
		}
		rel := indexedAssetPath("files", idx, file.Target, "file")
		if err := helpers.CreatePathAndCopy(src, filepath.Join(outputDir, rel)); err != nil {
			return fmt.Errorf("copying component file %d: %w", idx, err)
		}
		file.Source = componentSourcePath(component.Name, rel)
		file.ExtractPath = ""
	}
	return nil
}

func localizeDataInjections(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, tmpRoot string, component *v1alpha1.ZarfComponent) error {
	dataDir, err := pkgLayout.GetComponentDir(ctx, tmpRoot, component.Name, layout.DataComponentDir)
	if err != nil {
		return fmt.Errorf("reading data assets for component %s: %w", component.Name, err)
	}
	for idx := range component.DataInjections {
		data := &component.DataInjections[idx]
		src, err := sourceFromIndexedDir(dataDir, idx, data.Target.Path)
		if err != nil {
			return err
		}
		rel := indexedAssetPath("data", idx, data.Target.Path, "data")
		if err := helpers.CreatePathAndCopy(src, filepath.Join(outputDir, rel)); err != nil {
			return fmt.Errorf("copying data injection %d: %w", idx, err)
		}
		data.Source = componentSourcePath(component.Name, rel)
	}
	return nil
}

func indexedAssetPath(kind string, idx int, target, fallback string) string {
	name := filepath.Base(target)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = fmt.Sprintf("%s-%d", fallback, idx)
	}
	return filepath.ToSlash(filepath.Join(kind, strconv.Itoa(idx), name))
}

func sourceFromIndexedDir(root string, idx int, target string) (string, error) {
	candidate := filepath.Join(root, layout.ComponentFileRelPath(idx, target))
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	indexedPath := filepath.Join(root, strconv.Itoa(idx))
	if _, err := os.Stat(indexedPath); err == nil {
		return indexedPath, nil
	}
	return "", fmt.Errorf("unable to resolve indexed path %d in %s", idx, root)
}
