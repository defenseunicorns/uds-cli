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
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

func localizeManifests(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, tmpRoot string, component *v1alpha1.ZarfComponent) error {
	manifestDir, err := pkgLayout.GetComponentDir(ctx, tmpRoot, component.Name, layout.ManifestsComponentDir)
	if err != nil {
		return fmt.Errorf("reading manifest assets for component %s: %w", component.Name, err)
	}
	for mIdx := range component.Manifests {
		manifest := &component.Manifests[mIdx]
		localizedFiles := make([]string, 0, len(manifest.Files))
		for idx := range manifest.Files {
			src := filepath.Join(manifestDir, layout.ManifestFileName(manifest.Name, idx))
			rel := filepath.ToSlash(filepath.Join("manifests", manifest.Name, fmt.Sprintf("file-%d.yaml", idx)))
			if err := helpers.CreatePathAndCopy(src, filepath.Join(outputDir, rel)); err != nil {
				return fmt.Errorf("copying manifest %s file %d: %w", manifest.Name, idx, err)
			}
			localizedFiles = append(localizedFiles, componentSourcePath(component.Name, rel))
		}
		localizedKustomizations := make([]string, 0, len(manifest.Kustomizations))
		for idx := range manifest.Kustomizations {
			src := filepath.Join(manifestDir, layout.KustomizationFileName(manifest.Name, idx))
			rel := filepath.ToSlash(filepath.Join("manifests", manifest.Name, fmt.Sprintf("kustomization-%d", idx)))
			if err := helpers.CreatePathAndCopy(src, filepath.Join(outputDir, rel, "rendered.yaml")); err != nil {
				return fmt.Errorf("copying manifest kustomization %s %d: %w", manifest.Name, idx, err)
			}
			wrapper := filepath.Join(outputDir, rel, "kustomization.yaml")
			if err := os.WriteFile(wrapper, []byte("resources:\n  - rendered.yaml\n"), helpers.ReadWriteUser); err != nil {
				return fmt.Errorf("writing manifest kustomization wrapper %s %d: %w", manifest.Name, idx, err)
			}
			localizedKustomizations = append(localizedKustomizations, componentSourcePath(component.Name, rel))
		}
		manifest.Files = localizedFiles
		manifest.Kustomizations = localizedKustomizations
		manifest.KustomizeAllowAnyDirectory = false
		manifest.EnableKustomizePlugins = false
	}
	return nil
}

func localizeCharts(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, tmpRoot string, component *v1alpha1.ZarfComponent) error {
	chartDir, err := pkgLayout.GetComponentDir(ctx, tmpRoot, component.Name, layout.ChartsComponentDir)
	if err != nil {
		return fmt.Errorf("reading chart assets for component %s: %w", component.Name, err)
	}
	valuesDir, err := pkgLayout.GetComponentDir(ctx, tmpRoot, component.Name, layout.ValuesComponentDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reading values assets for component %s: %w", component.Name, err)
	}

	for idx := range component.Charts {
		chart := &component.Charts[idx]
		archiveName := layout.ChartArchiveName(chart.Name, chart.Version)
		src := filepath.Join(chartDir, archiveName)
		rel := filepath.ToSlash(filepath.Join("charts", fmt.Sprintf("%d-%s", idx, archiveName)))
		if err := helpers.CreatePathAndCopy(src, filepath.Join(outputDir, rel)); err != nil {
			return fmt.Errorf("copying chart %s: %w", chart.Name, err)
		}
		chart.LocalPath = componentSourcePath(component.Name, rel)
		chart.URL = ""
		chart.RepoName = ""
		chart.GitPath = ""

		for valueIdx := range chart.ValuesFiles {
			localized, err := localizeChartValues(valuesDir, outputDir, component.Name, *chart, valueIdx, valueIdx, chart.ValuesFiles[valueIdx], false)
			if err != nil {
				return err
			}
			chart.ValuesFiles[valueIdx] = localized
		}
		for valueIdx := range chart.TemplatedValuesFiles {
			globalIdx := len(chart.ValuesFiles) + valueIdx
			localized, err := localizeChartValues(valuesDir, outputDir, component.Name, *chart, valueIdx, globalIdx, chart.TemplatedValuesFiles[valueIdx], true)
			if err != nil {
				return err
			}
			chart.TemplatedValuesFiles[valueIdx] = localized
		}
	}
	return nil
}

func localizeChartValues(valuesDir, outputDir, componentName string, chart v1alpha1.ZarfChart, valueIdx, globalIdx int, original string, templated bool) (string, error) {
	src := filepath.Join(valuesDir, layout.ChartValuesFileName(chart.Name, chart.Version, globalIdx))
	base := portableAssetName(original, "values.yaml")
	prefix := "values"
	if templated {
		prefix = "templated-values"
	}
	rel := filepath.ToSlash(filepath.Join("values", chart.Name, fmt.Sprintf("%s-%d-%s", prefix, valueIdx, base)))
	if err := helpers.CreatePathAndCopy(src, filepath.Join(outputDir, rel)); err != nil {
		return "", fmt.Errorf("copying chart values for %s: %w", chart.Name, err)
	}
	return componentSourcePath(componentName, rel), nil
}

func portableAssetName(source, fallback string) string {
	name := filepath.Base(source)
	if helpers.IsURL(source) {
		if urlName, err := helpers.ExtractBasePathFromURL(source); err == nil {
			name = urlName
		}
	}
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '-'
		}
		return r
	}, name)
	name = strings.TrimRight(name, ". ")
	if name == "" || name == "." {
		return fallback
	}

	stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if isWindowsReservedName(stem) {
		name = "_" + name
	}
	return name
}

func isWindowsReservedName(stem string) bool {
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9'
}
