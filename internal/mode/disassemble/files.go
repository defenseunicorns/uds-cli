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
	return localizeIndexedAssets(ctx, pkgLayout, outputDir, tmpRoot, component.Name, layout.FilesComponentDir, "files", "file", component.Files,
		func(file *v1alpha1.ZarfFile) string { return file.Target },
		func(file *v1alpha1.ZarfFile, source string) {
			file.Source = source
			file.ExtractPath = ""
		})
}

func localizeDataInjections(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, tmpRoot string, component *v1alpha1.ZarfComponent) error {
	return localizeIndexedAssets(ctx, pkgLayout, outputDir, tmpRoot, component.Name, layout.DataComponentDir, "data", "data", component.DataInjections,
		func(data *v1alpha1.ZarfDataInjection) string { return data.Target.Path },
		func(data *v1alpha1.ZarfDataInjection, source string) { data.Source = source })
}

func localizeIndexedAssets[T any](ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, tmpRoot, componentName string, componentDir layout.ComponentDir, kind, fallback string, assets []T, target func(*T) string, update func(*T, string)) error {
	assetDir, err := pkgLayout.GetComponentDir(ctx, tmpRoot, componentName, componentDir)
	if err != nil {
		return fmt.Errorf("reading %s assets for component %s: %w", kind, componentName, err)
	}
	for idx := range assets {
		asset := &assets[idx]
		targetPath := target(asset)
		src, err := sourceFromIndexedDir(assetDir, idx, targetPath)
		if err != nil {
			return err
		}
		rel := indexedAssetPath(kind, idx, targetPath, fallback)
		if err := helpers.CreatePathAndCopy(src, filepath.Join(outputDir, rel)); err != nil {
			return fmt.Errorf("copying component %s %d: %w", kind, idx, err)
		}
		update(asset, componentSourcePath(componentName, rel))
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
