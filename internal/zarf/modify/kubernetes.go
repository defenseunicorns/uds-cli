// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package modify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	goyaml "github.com/goccy/go-yaml"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	chartarchive "helm.sh/helm/v4/pkg/chart/loader/archive"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
)

func localizeManifests(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, tmpRoot string, component *api.Component) error {
	manifestDir, err := pkgLayout.GetComponentDir(ctx, tmpRoot, component.Name, layout.ManifestsComponentDir)
	if err != nil {
		return fmt.Errorf("reading manifest assets for component %s: %w", component.Name, err)
	}
	for mIdx := range component.Manifests {
		manifest := &component.Manifests[mIdx]
		localizedFiles := make([]string, 0, len(manifest.Files))
		for idx := range manifest.Files {
			name, err := sourceBaseName(manifest.Files[idx], "manifest.yaml")
			if err != nil {
				return fmt.Errorf("resolving manifest %s file %d name: %w", manifest.Name, idx, err)
			}
			src := filepath.Join(manifestDir, layout.ManifestFileName(manifest.Name, idx))
			rel := filepath.ToSlash(filepath.Join("manifests", manifest.Name, fmt.Sprintf("%d-%s", idx, name)))
			if err := helpers.CreatePathAndCopy(src, filepath.Join(outputDir, rel)); err != nil {
				return fmt.Errorf("copying manifest %s file %d: %w", manifest.Name, idx, err)
			}
			localizedFiles = append(localizedFiles, componentSourcePath(component.Name, rel))
		}
		localizedKustomizations := make([]string, 0, len(manifest.Kustomize.Files))
		for idx := range manifest.Kustomize.Files {
			name, err := sourceBaseName(manifest.Kustomize.Files[idx], "kustomization")
			if err != nil {
				return fmt.Errorf("resolving manifest %s kustomization %d name: %w", manifest.Name, idx, err)
			}
			src := filepath.Join(manifestDir, layout.KustomizationFileName(manifest.Name, idx))
			rel := filepath.ToSlash(filepath.Join("manifests", manifest.Name, fmt.Sprintf("%d-%s", idx, name)))
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
		manifest.Kustomize.Files = localizedKustomizations
		manifest.Kustomize.AllowAnyDirectory = false
		manifest.Kustomize.EnablePlugins = false
	}
	return nil
}

func localizeCharts(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, tmpRoot string, component *api.Component) error {
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
		archiveName := layout.ChartArchiveName(chart.Name, chart.LegacyVersion)
		src := filepath.Join(chartDir, archiveName)
		rel := filepath.ToSlash(filepath.Join("charts", chart.Name))
		if err := extractChartArchive(src, filepath.Join(outputDir, rel)); err != nil {
			return fmt.Errorf("extracting chart %s: %w", chart.Name, err)
		}
		chart.HelmRepository = nil
		chart.Git = nil
		chart.OCI = nil
		chart.Local = &api.LocalSource{Path: componentSourcePath(component.Name, rel)}

		for valueIdx := range chart.ValuesFiles {
			localized, err := localizeChartValues(valuesDir, outputDir, component.Name, *chart, valueIdx, chart.ValuesFiles[valueIdx].Path)
			if err != nil {
				return err
			}
			chart.ValuesFiles[valueIdx].Path = localized
		}
	}
	return nil
}

func localizeChartValues(valuesDir, outputDir, componentName string, chart api.Chart, idx int, original string) (string, error) {
	src := filepath.Join(valuesDir, layout.ChartValuesFileName(chart.Name, chart.LegacyVersion, idx))
	base, err := sourceBaseName(original, "values.yaml")
	if err != nil {
		return "", fmt.Errorf("resolving chart values name for %s: %w", chart.Name, err)
	}
	rel := filepath.ToSlash(filepath.Join("values", chart.Name, fmt.Sprintf("%d-%s", idx, base)))
	if err := helpers.CreatePathAndCopy(src, filepath.Join(outputDir, rel)); err != nil {
		return "", fmt.Errorf("copying chart values for %s: %w", chart.Name, err)
	}
	return componentSourcePath(componentName, rel), nil
}

func extractChartArchive(source, destination string) (err error) {
	archiveFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, archiveFile.Close())
	}()

	files, err := chartarchive.LoadArchiveFiles(archiveFile)
	if err != nil {
		return err
	}
	for _, file := range files {
		rel := filepath.FromSlash(file.Name)
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("chart contains invalid path %q", file.Name)
		}
		data := file.Data
		if name, ok := chartRootMetadata(file.Name); ok {
			if name == "Chart.lock" || name == "requirements.lock" {
				continue
			}
			data, err = localizeChartDependencies(name, data)
			if err != nil {
				return err
			}
		}
		path := filepath.Join(destination, rel)
		if err := os.MkdirAll(filepath.Dir(path), helpers.ReadWriteExecuteUser); err != nil {
			return fmt.Errorf("creating chart directory for %s: %w", file.Name, err)
		}
		if err := os.WriteFile(path, data, helpers.ReadWriteUser); err != nil {
			return fmt.Errorf("writing chart file %s: %w", file.Name, err)
		}
	}
	return nil
}

func chartRootMetadata(name string) (string, bool) {
	parts := strings.Split(name, "/")
	metadata := parts[len(parts)-1]
	switch metadata {
	case "Chart.yaml", "Chart.lock", "requirements.yaml", "requirements.lock":
	default:
		return "", false
	}

	directories := parts[:len(parts)-1]
	if len(directories)%2 != 0 {
		return "", false
	}
	for idx := 0; idx < len(directories); idx += 2 {
		if directories[idx] != "charts" || directories[idx+1] == "" {
			return "", false
		}
	}
	return metadata, true
}

type chartRequirements struct {
	Dependencies []*chartv2.Dependency `yaml:"dependencies,omitempty"`
}

func localizeChartDependencies(name string, data []byte) ([]byte, error) {
	var definition any
	switch name {
	case "Chart.yaml":
		definition = &chartv2.Metadata{}
	case "requirements.yaml":
		definition = &chartRequirements{}
	default:
		return data, nil
	}
	if err := goyaml.Unmarshal(data, definition); err != nil {
		return nil, fmt.Errorf("reading %s: %w", name, err)
	}

	var dependencies []*chartv2.Dependency
	switch typed := definition.(type) {
	case *chartv2.Metadata:
		dependencies = typed.Dependencies
	case *chartRequirements:
		dependencies = typed.Dependencies
	}
	if len(dependencies) == 0 {
		return data, nil
	}
	for _, dependency := range dependencies {
		dependency.Repository = ""
	}
	contents, err := goyaml.MarshalWithOptions(definition, goyaml.IndentSequence(true))
	if err != nil {
		return nil, fmt.Errorf("writing %s: %w", name, err)
	}
	return contents, nil
}

func sourceBaseName(source, fallback string) (string, error) {
	base := filepath.Base(source)
	if helpers.IsURL(source) {
		var err error
		base, err = helpers.ExtractBasePathFromURL(source)
		if err != nil {
			return "", err
		}
	}
	if base == "" || base == "." || base == string(filepath.Separator) {
		return fallback, nil
	}
	return base, nil
}
