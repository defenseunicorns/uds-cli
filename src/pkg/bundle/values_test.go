// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
)

func TestSetValueAtPath(t *testing.T) {
	tests := []struct {
		name     string
		initial  ZarfValues
		path     string
		value    interface{}
		expected ZarfValues
		wantErr  bool
	}{
		{
			name:     "simple path",
			initial:  make(ZarfValues),
			path:     ".app",
			value:    "myapp",
			expected: ZarfValues{"app": "myapp"},
			wantErr:  false,
		},
		{
			name:     "nested path",
			initial:  make(ZarfValues),
			path:     ".app.name",
			value:    "myapp",
			expected: ZarfValues{"app": map[string]any{"name": "myapp"}},
			wantErr:  false,
		},
		{
			name:     "deeply nested path",
			initial:  make(ZarfValues),
			path:     ".app.config.database.host",
			value:    "localhost",
			expected: ZarfValues{"app": map[string]any{"config": map[string]any{"database": map[string]any{"host": "localhost"}}}},
			wantErr:  false,
		},
		{
			name:    "invalid path - no dot prefix",
			initial: make(ZarfValues),
			path:    "app.name",
			value:   "myapp",
			wantErr: true,
		},
		{
			name:    "invalid path - empty",
			initial: make(ZarfValues),
			path:    "",
			value:   "myapp",
			wantErr: true,
		},
		{
			name:     "override existing value",
			initial:  ZarfValues{"app": "old"},
			path:     ".app",
			value:    "new",
			expected: ZarfValues{"app": "new"},
			wantErr:  false,
		},
		{
			name:     "numeric value",
			initial:  make(ZarfValues),
			path:     ".replicas",
			value:    3,
			expected: ZarfValues{"replicas": 3},
			wantErr:  false,
		},
		{
			name:     "map value",
			initial:  make(ZarfValues),
			path:     ".resources",
			value:    map[string]any{"cpu": "100m", "memory": "128Mi"},
			expected: ZarfValues{"resources": map[string]any{"cpu": "100m", "memory": "128Mi"}},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setValueAtPath(tt.initial, tt.path, tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, tt.initial)
			}
		})
	}
}

func TestDeepMergeValues(t *testing.T) {
	tests := []struct {
		name     string
		dst      ZarfValues
		src      ZarfValues
		expected ZarfValues
	}{
		{
			name:     "merge into empty",
			dst:      make(ZarfValues),
			src:      ZarfValues{"app": "myapp"},
			expected: ZarfValues{"app": "myapp"},
		},
		{
			name:     "override simple value",
			dst:      ZarfValues{"app": "old"},
			src:      ZarfValues{"app": "new"},
			expected: ZarfValues{"app": "new"},
		},
		{
			name: "merge nested maps",
			dst: ZarfValues{
				"app": map[string]any{
					"name":    "myapp",
					"version": "1.0",
				},
			},
			src: ZarfValues{
				"app": map[string]any{
					"version": "2.0",
					"env":     "prod",
				},
			},
			expected: ZarfValues{
				"app": map[string]any{
					"name":    "myapp",
					"version": "2.0",
					"env":     "prod",
				},
			},
		},
		{
			name: "add new keys",
			dst: ZarfValues{
				"app": "myapp",
			},
			src: ZarfValues{
				"replicas": 3,
			},
			expected: ZarfValues{
				"app":      "myapp",
				"replicas": 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deepMergeValues(tt.dst, tt.src)
			require.Equal(t, tt.expected, tt.dst)
		})
	}
}

func TestParseValuesFile(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "values-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("parse valid yaml file", func(t *testing.T) {
		content := `
app:
  name: myapp
  replicas: 3
  config:
    debug: true
`
		filePath := filepath.Join(tmpDir, "values.yaml")
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)

		vals, err := parseValuesFile(filePath)
		require.NoError(t, err)
		require.NotNil(t, vals)

		app, ok := vals["app"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "myapp", app["name"])
		// YAML parser returns numbers as uint64
		require.Equal(t, uint64(3), app["replicas"])
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := parseValuesFile(filepath.Join(tmpDir, "nonexistent.yaml"))
		require.Error(t, err)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		content := `invalid: yaml: content: [broken`
		filePath := filepath.Join(tmpDir, "invalid.yaml")
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)

		_, err = parseValuesFile(filePath)
		require.Error(t, err)
	})
}

func TestResolveValuesFilePath(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "values-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "values.yaml")
	err = os.WriteFile(testFile, []byte("test: true"), 0644)
	require.NoError(t, err)

	t.Run("resolve relative path", func(t *testing.T) {
		resolved, err := resolveValuesFilePath("values.yaml", tmpDir)
		require.NoError(t, err)
		require.Equal(t, testFile, resolved)
	})

	t.Run("resolve absolute path", func(t *testing.T) {
		resolved, err := resolveValuesFilePath(testFile, "/some/other/dir")
		require.NoError(t, err)
		require.Equal(t, testFile, resolved)
	})

	t.Run("file not found - relative", func(t *testing.T) {
		_, err := resolveValuesFilePath("nonexistent.yaml", tmpDir)
		require.Error(t, err)
	})

	t.Run("file not found - absolute", func(t *testing.T) {
		_, err := resolveValuesFilePath("/nonexistent/path/values.yaml", tmpDir)
		require.Error(t, err)
	})
}

func TestLoadPackageValuesPrecedence(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "values-precedence-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create bundle values file
	bundleValuesFile := filepath.Join(tmpDir, "bundle-values.yaml")
	err = os.WriteFile(bundleValuesFile, []byte(`
app:
  name: "from-bundle-file"
  environment: "from-bundle-file"
  replicas: 1
  fromBundleFile: true
`), 0644)
	require.NoError(t, err)

	// Create config values file
	configValuesFile := filepath.Join(tmpDir, "config-values.yaml")
	err = os.WriteFile(configValuesFile, []byte(`
app:
  environment: "from-config-file"
  fromConfigFile: true
`), 0644)
	require.NoError(t, err)

	t.Run("bundle values.set overrides values.files", func(t *testing.T) {
		b := &Bundle{
			cfg: &types.BundleConfig{
				DeployOpts: types.BundleDeployOptions{
					Source: filepath.Join(tmpDir, "bundle.tar.zst"),
				},
			},
		}

		pkg := types.Package{
			Name: "test-pkg",
			Values: &types.PackageValues{
				Files: []string{"bundle-values.yaml"},
				Set: map[string]interface{}{
					".app.replicas": 5,
				},
			},
		}

		vals, err := b.loadPackageValues(t.Context(), pkg, map[string]interface{}{})
		require.NoError(t, err)

		app := vals["app"].(map[string]any)
		require.Equal(t, "from-bundle-file", app["name"]) // from file
		require.Equal(t, 5, app["replicas"])              // overridden by set
		require.Equal(t, true, app["fromBundleFile"])     // preserved from file
	})

	t.Run("bundle values.variables overrides values.set", func(t *testing.T) {
		b := &Bundle{
			cfg: &types.BundleConfig{
				DeployOpts: types.BundleDeployOptions{
					Source: filepath.Join(tmpDir, "bundle.tar.zst"),
				},
			},
		}

		pkg := types.Package{
			Name: "test-pkg",
			Values: &types.PackageValues{
				Files: []string{"bundle-values.yaml"},
				Set: map[string]interface{}{
					".app.replicas": 5,
				},
				Variables: []types.BundleValuesVariable{
					{Name: "REPLICAS", Path: ".app.replicas", Default: 10},
				},
			},
		}

		// Without UDS variable set, uses default from variables
		vals, err := b.loadPackageValues(t.Context(), pkg, map[string]interface{}{})
		require.NoError(t, err)
		app := vals["app"].(map[string]any)
		require.Equal(t, 10, app["replicas"]) // from variables default, overrides set

		// With UDS variable set (integer preserved, not stringified)
		vals, err = b.loadPackageValues(t.Context(), pkg, map[string]interface{}{"REPLICAS": 20})
		require.NoError(t, err)
		app = vals["app"].(map[string]any)
		require.Equal(t, 20, app["replicas"]) // from UDS variable, type preserved
	})

	t.Run("config values override bundle values", func(t *testing.T) {
		b := &Bundle{
			cfg: &types.BundleConfig{
				DeployOpts: types.BundleDeployOptions{
					Source: filepath.Join(tmpDir, "bundle.tar.zst"),
					Config: filepath.Join(tmpDir, "uds-config.yaml"),
					PackageValues: map[string]types.PackageValuesConfig{
						"test-pkg": {
							Files: []string{"config-values.yaml"},
							Set: map[string]interface{}{
								".app.name": "from-config-set",
							},
						},
					},
				},
			},
		}

		pkg := types.Package{
			Name: "test-pkg",
			Values: &types.PackageValues{
				Files: []string{"bundle-values.yaml"},
				Set: map[string]interface{}{
					".app.replicas": 5,
				},
			},
		}

		vals, err := b.loadPackageValues(t.Context(), pkg, map[string]interface{}{})
		require.NoError(t, err)

		app := vals["app"].(map[string]any)
		require.Equal(t, "from-config-set", app["name"])         // highest: config set
		require.Equal(t, "from-config-file", app["environment"]) // config file overrides bundle
		require.Equal(t, 5, app["replicas"])                     // from bundle set (not in config)
		require.Equal(t, true, app["fromBundleFile"])            // preserved from bundle file
		require.Equal(t, true, app["fromConfigFile"])            // added by config file
	})

	t.Run("deep merge preserves nested values", func(t *testing.T) {
		// Create a file with nested structure
		nestedFile := filepath.Join(tmpDir, "nested-values.yaml")
		err = os.WriteFile(nestedFile, []byte(`
app:
  config:
    database:
      host: "localhost"
      port: 5432
    cache:
      enabled: true
`), 0644)
		require.NoError(t, err)

		b := &Bundle{
			cfg: &types.BundleConfig{
				DeployOpts: types.BundleDeployOptions{
					Source: filepath.Join(tmpDir, "bundle.tar.zst"),
				},
			},
		}

		pkg := types.Package{
			Name: "test-pkg",
			Values: &types.PackageValues{
				Files: []string{"nested-values.yaml"},
				Set: map[string]interface{}{
					".app.config.database.host": "production-db",
				},
			},
		}

		vals, err := b.loadPackageValues(t.Context(), pkg, map[string]interface{}{})
		require.NoError(t, err)

		app := vals["app"].(map[string]any)
		config := app["config"].(map[string]any)
		database := config["database"].(map[string]any)
		cache := config["cache"].(map[string]any)

		require.Equal(t, "production-db", database["host"]) // overridden
		require.Equal(t, uint64(5432), database["port"])    // preserved
		require.Equal(t, true, cache["enabled"])            // preserved (different branch)
	})
}

func TestLoadPackageValuesNoConfig(t *testing.T) {
	t.Run("returns empty map when no values configured", func(t *testing.T) {
		b := &Bundle{
			cfg: &types.BundleConfig{
				DeployOpts: types.BundleDeployOptions{
					Source: "/some/path/bundle.tar.zst",
				},
			},
		}

		pkg := types.Package{
			Name:   "test-pkg",
			Values: nil, // no values configured
		}

		vals, err := b.loadPackageValues(t.Context(), pkg, map[string]interface{}{})
		require.NoError(t, err)
		require.Empty(t, vals)
	})
}

func TestLoadPackageValuesComplexObjects(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "values-complex-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("complex object from variable is preserved", func(t *testing.T) {
		b := &Bundle{
			cfg: &types.BundleConfig{
				DeployOpts: types.BundleDeployOptions{
					Source: filepath.Join(tmpDir, "bundle.tar.zst"),
				},
			},
		}

		pkg := types.Package{
			Name: "test-pkg",
			Values: &types.PackageValues{
				Variables: []types.BundleValuesVariable{
					{Name: "RESOURCES", Path: ".app.resources"},
				},
			},
		}

		complexValue := map[string]interface{}{
			"replicas": 3,
			"limits": map[string]interface{}{
				"cpu":    "500m",
				"memory": "1Gi",
			},
		}

		vals, err := b.loadPackageValues(t.Context(), pkg, map[string]interface{}{
			"RESOURCES": complexValue,
		})
		require.NoError(t, err)

		app, ok := vals["app"].(map[string]any)
		require.True(t, ok, "expected app to be a map")

		resources, ok := app["resources"].(map[string]interface{})
		require.True(t, ok, "expected resources to be a map, not a string")

		require.Equal(t, 3, resources["replicas"])
		limits, ok := resources["limits"].(map[string]interface{})
		require.True(t, ok, "expected limits to be a map")
		require.Equal(t, "500m", limits["cpu"])
		require.Equal(t, "1Gi", limits["memory"])
	})

	t.Run("array from variable is preserved", func(t *testing.T) {
		b := &Bundle{
			cfg: &types.BundleConfig{
				DeployOpts: types.BundleDeployOptions{
					Source: filepath.Join(tmpDir, "bundle.tar.zst"),
				},
			},
		}

		pkg := types.Package{
			Name: "test-pkg",
			Values: &types.PackageValues{
				Variables: []types.BundleValuesVariable{
					{Name: "ADMIN_GROUPS", Path: ".sso.adminGroups"},
				},
			},
		}

		arrayValue := []interface{}{"/GitLab Admin", "/UDS Core/Admin"}

		vals, err := b.loadPackageValues(t.Context(), pkg, map[string]interface{}{
			"ADMIN_GROUPS": arrayValue,
		})
		require.NoError(t, err)

		sso, ok := vals["sso"].(map[string]any)
		require.True(t, ok, "expected sso to be a map")

		adminGroups, ok := sso["adminGroups"].([]interface{})
		require.True(t, ok, "expected adminGroups to be an array, not a string")
		require.Len(t, adminGroups, 2)
		require.Equal(t, "/GitLab Admin", adminGroups[0])
	})

	t.Run("string variable still works", func(t *testing.T) {
		b := &Bundle{
			cfg: &types.BundleConfig{
				DeployOpts: types.BundleDeployOptions{
					Source: filepath.Join(tmpDir, "bundle.tar.zst"),
				},
			},
		}

		pkg := types.Package{
			Name: "test-pkg",
			Values: &types.PackageValues{
				Variables: []types.BundleValuesVariable{
					{Name: "APP_NAME", Path: ".app.name"},
				},
			},
		}

		vals, err := b.loadPackageValues(t.Context(), pkg, map[string]interface{}{
			"APP_NAME": "my-app",
		})
		require.NoError(t, err)

		app, ok := vals["app"].(map[string]any)
		require.True(t, ok, "expected app to be a map")
		require.Equal(t, "my-app", app["name"])
	})
}
