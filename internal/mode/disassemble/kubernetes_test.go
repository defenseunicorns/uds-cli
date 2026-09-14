// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	goyaml "github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	chartv3 "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
)

func TestExtractChartArchiveLocalizesExpandedDependencies(t *testing.T) {
	dependency := &chartv3.Dependency{
		Name:       "child",
		Version:    "1.x",
		Repository: "https://invalid.example.com/charts",
		Alias:      "renamed-child",
	}
	parent := &chartv3.Chart{
		Metadata: &chartv3.Metadata{
			APIVersion:   chartv3.APIVersionV2,
			Name:         "parent",
			Version:      "1.0.0",
			Dependencies: []*chartv3.Dependency{dependency},
		},
		Lock: &chartv3.Lock{
			Generated:    time.Now(),
			Digest:       "stale",
			Dependencies: []*chartv3.Dependency{dependency},
		},
		Templates: []*chartv3.File{{Name: "templates/Chart.lock", Data: []byte("template lock")}},
		Files:     []*chartv3.File{{Name: "docs/requirements.yaml", Data: []byte("dependencies: application data")}},
	}
	childDependency := &chartv3.Dependency{Name: "grandchild", Version: "2.x", Repository: "https://invalid.example.com/nested"}
	child := &chartv3.Chart{
		Metadata: &chartv3.Metadata{
			APIVersion:   chartv3.APIVersionV2,
			Name:         "child",
			Version:      "1.2.3",
			Dependencies: []*chartv3.Dependency{childDependency},
		},
		Lock: &chartv3.Lock{
			Generated:    time.Now(),
			Digest:       "nested-stale",
			Dependencies: []*chartv3.Dependency{childDependency},
		},
	}
	child.AddDependency(&chartv3.Chart{Metadata: &chartv3.Metadata{
		APIVersion: chartv3.APIVersionV2,
		Name:       "grandchild",
		Version:    "2.1.0",
	}})
	parent.AddDependency(child)
	archivePath, err := chartutil.Save(parent, t.TempDir())
	require.NoError(t, err)

	outputDir := t.TempDir()
	require.NoError(t, extractChartArchive(archivePath, outputDir))
	require.NoFileExists(t, filepath.Join(outputDir, "Chart.lock"))
	require.FileExists(t, filepath.Join(outputDir, "charts", "child", "Chart.yaml"))
	require.NoFileExists(t, filepath.Join(outputDir, "charts", "child", "Chart.lock"))
	require.FileExists(t, filepath.Join(outputDir, "charts", "child", "charts", "grandchild", "Chart.yaml"))
	templateLock, err := os.ReadFile(filepath.Join(outputDir, "templates", "Chart.lock"))
	require.NoError(t, err)
	require.Equal(t, "template lock", string(templateLock))
	applicationRequirements, err := os.ReadFile(filepath.Join(outputDir, "docs", "requirements.yaml"))
	require.NoError(t, err)
	require.Equal(t, "dependencies: application data", string(applicationRequirements))

	contents, err := os.ReadFile(filepath.Join(outputDir, "Chart.yaml"))
	require.NoError(t, err)
	var metadata chartv3.Metadata
	require.NoError(t, goyaml.Unmarshal(contents, &metadata))
	require.Len(t, metadata.Dependencies, 1)
	require.Empty(t, metadata.Dependencies[0].Repository)
	require.Equal(t, "renamed-child", metadata.Dependencies[0].Alias)
	contents, err = os.ReadFile(filepath.Join(outputDir, "charts", "child", "Chart.yaml"))
	require.NoError(t, err)
	var childMetadata chartv3.Metadata
	require.NoError(t, goyaml.Unmarshal(contents, &childMetadata))
	require.Len(t, childMetadata.Dependencies, 1)
	require.Empty(t, childMetadata.Dependencies[0].Repository)

	loaded, err := loader.LoadDir(outputDir)
	require.NoError(t, err)
	require.Len(t, loaded.Dependencies(), 1)
	require.Equal(t, "child", loaded.Dependencies()[0].Name())
}

func TestLocalizeChartDependenciesV1(t *testing.T) {
	contents := []byte(`dependencies:
  - name: child
    version: 1.x
    repository: https://invalid.example.com/charts
    alias: renamed-child
`)

	localized, err := localizeChartDependencies("requirements.yaml", contents)
	require.NoError(t, err)
	var requirements chartRequirements
	require.NoError(t, goyaml.Unmarshal(localized, &requirements))
	require.Len(t, requirements.Dependencies, 1)
	require.Empty(t, requirements.Dependencies[0].Repository)
	require.Equal(t, "renamed-child", requirements.Dependencies[0].Alias)
}
