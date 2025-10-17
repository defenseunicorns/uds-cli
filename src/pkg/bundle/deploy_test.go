// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/uds-cli/src/types/chartvariable"
	"github.com/defenseunicorns/uds-cli/src/types/valuesources"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/state"
	"helm.sh/helm/v3/pkg/cli/values"
)

type ConfigVariables map[string]map[string]interface{}
type ConfigSharedVariables map[string]interface{}
type SetVariables map[string]string
type BundleExportVars map[string]map[string]string

func newTestBundle(variables ConfigVariables, sharedVariables ConfigSharedVariables, setVariables SetVariables, udsConfigFile string, udsBundleFile string) Bundle {
	cfg := &types.BundleConfig{
		DeployOpts: types.BundleDeployOptions{
			Variables:       variables,
			SharedVariables: sharedVariables,
			SetVariables:    setVariables,
			Config:          udsConfigFile,
			Source:          udsBundleFile,
		},
	}
	return Bundle{
		cfg: cfg,
	}
}

func newTestPkg(pkgName string, componentName string, chartName string, overVar types.BundleChartVariable) types.Package {
	return types.Package{Name: pkgName,
		Overrides: map[string]map[string]types.BundleChartOverrides{componentName: {chartName: {Variables: []types.BundleChartVariable{
			overVar,
		}}}}}
}

func TestLoadVariablesPrecedence(t *testing.T) {

	bundleExportVars := BundleExportVars{
		"barPkg": {
			"foo": "exported from another pkg",
		},
		"bazPkg": {
			"foo": "imported from a specific pkg",
		},
	}

	testPkg := types.Package{
		Name: "fooPkg",
		Imports: []types.BundleVariableImport{
			{
				Name:    "foo",
				Package: "bazPkg",
			},
		},
	}

	testCases := []struct {
		name             string
		description      string
		pkg              types.Package
		bundle           Bundle
		bundleExportVars BundleExportVars
		loadEnvVar       bool
		expectedPkgVars  zarfVarData
	}{
		{
			name:       "--set flag precedence",
			loadEnvVar: true,
			pkg:        testPkg,
			bundle: newTestBundle(
				ConfigVariables{
					"fooPkg": {
						"foo": "set from variables key in uds-config.yaml",
					},
				},
				// set from uds-config.yaml
				ConfigSharedVariables{
					"foo": "set from shared key in uds-config.yaml",
				},
				SetVariables{
					"foo": "set using --set flag",
				},
				"",
				"",
			),
			bundleExportVars: bundleExportVars,
			expectedPkgVars: zarfVarData{
				"FOO": "set using --set flag",
			},
		},
		{
			name:       "env var precedence",
			loadEnvVar: true,
			pkg:        testPkg,
			bundle: newTestBundle(
				ConfigVariables{
					"fooPkg": {
						"foo": "set from variables key in uds-config.yaml",
					},
				},
				// set from uds-config.yaml
				ConfigSharedVariables{
					"foo": "set from shared key in uds-config.yaml",
				},
				nil,
				"",
				"",
			),
			bundleExportVars: bundleExportVars,
			expectedPkgVars: zarfVarData{
				"FOO": "set using env var",
			},
		},
		{
			name: "uds-config variables key precedence",
			pkg:  testPkg,
			bundle: newTestBundle(
				ConfigVariables{
					"fooPkg": {
						"foo": "set from variables key in uds-config.yaml",
					},
				},
				// set from uds-config.yaml
				ConfigSharedVariables{
					"foo": "set from shared key in uds-config.yaml",
				},
				nil,
				"",
				"",
			),
			bundleExportVars: bundleExportVars,
			expectedPkgVars: zarfVarData{
				"FOO": "set from variables key in uds-config.yaml",
			},
		},
		{
			name: "uds-config shared key precedence",
			pkg:  testPkg,
			bundle: newTestBundle(
				nil,
				ConfigSharedVariables{
					"foo": "set from shared key in uds-config.yaml",
				},
				nil,
				"",
				"",
			),
			bundleExportVars: bundleExportVars,
			expectedPkgVars: zarfVarData{
				"FOO": "set from shared key in uds-config.yaml",
			},
		},
		{
			name:             "uds-config shared key precedence",
			pkg:              testPkg,
			bundle:           newTestBundle(nil, nil, nil, "", ""),
			bundleExportVars: bundleExportVars,
			expectedPkgVars: zarfVarData{
				"FOO": "imported from a specific pkg",
			},
		},
		{
			name: "uds-config global export precedence",
			pkg: types.Package{
				Name: "fooPkg",
			},
			bundle: newTestBundle(nil, nil, nil, "", ""),
			bundleExportVars: BundleExportVars{
				"barPkg": {
					"foo": "exported from another pkg",
				},
			},
			expectedPkgVars: zarfVarData{
				"FOO": "exported from another pkg",
			},
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// unset uds run vars that get applied automatically when doing 'uds run' so it doesn't get in the way
			os.Unsetenv("UDS_ARCH")
			os.Unsetenv("UDS_NO_PROGRESS")

			// Set for select test cases to test precedence of env vars
			os.Unsetenv("UDS_FOO")
			if tc.loadEnvVar {
				os.Setenv("UDS_FOO", "set using env var")
			}
			actualPkgVars, _ := tc.bundle.loadVariables(tc.pkg, tc.bundleExportVars)
			require.Equal(t, tc.expectedPkgVars, actualPkgVars)
		})
	}
}

func TestFileVariableHandlers(t *testing.T) {
	cwd, _ := os.Getwd()
	const (
		componentName = "test-component"
		chartName     = "test-chart"
		pkgName       = "test-package"
		varName       = "CERT"
		path          = "test.Cert"
		relativePath  = "../../../src/test/bundles/07-helm-overrides/variable-files/"
	)

	testCases := []struct {
		name         string
		Bundle       Bundle
		loadEnv      bool
		requireNoErr bool
		expected     string
		bundleVars   *[]types.BundleChartVariable
	}{
		{
			name: "with --set",
			Bundle: newTestBundle(
				nil,
				nil,
				SetVariables{
					varName: fmt.Sprintf("%s/test.cert", relativePath),
				},
				"",
				"",
			),
			bundleVars: &[]types.BundleChartVariable{
				{
					Name:        varName,
					Path:        path,
					Type:        chartvariable.File,
					Description: "set the var from cli, so source path is current working directory (eg. /home/user/repos/uds-cli/...)",
				},
			},
			requireNoErr: true,
			expected:     fmt.Sprintf("%s=%s", path, filepath.Join(cwd, fmt.Sprintf("%s/test.cert", relativePath))),
		},
		{
			name:   "with UDS_VAR",
			Bundle: newTestBundle(nil, nil, nil, "", ""),
			bundleVars: &[]types.BundleChartVariable{
				{
					Name:        varName,
					Path:        path,
					Type:        chartvariable.File,
					Description: "set the var from env, so source path is current working directory (eg. /home/user/repos/uds-cli/...)",
				},
			},
			loadEnv:      true,
			requireNoErr: true,
			expected:     fmt.Sprintf("%s=%s", path, filepath.Join(cwd, fmt.Sprintf("%s/test.cert", relativePath))),
		},
		{
			name: "with Config",
			Bundle: newTestBundle(
				ConfigVariables{
					pkgName: {
						varName: "test.cert",
					},
				},
				nil,
				nil,
				fmt.Sprintf("%s/uds-config.yaml", relativePath),
				"",
			),
			bundleVars: &[]types.BundleChartVariable{
				{
					Name:        varName,
					Path:        path,
					Type:        chartvariable.File,
					Description: "set the var from config, so source path is config directory",
				},
			},
			requireNoErr: true,
			expected:     fmt.Sprintf("%s=%s", path, fmt.Sprintf("%stest.cert", relativePath)),
		},
		{
			name: "with Bundle",
			Bundle: newTestBundle(
				nil,
				nil,
				nil,
				"",
				fmt.Sprintf("%s/uds-bundle-helm-overrides-amd64-0.0.1.tar.zst", relativePath),
			),
			bundleVars: &[]types.BundleChartVariable{
				{
					Name:        varName,
					Path:        path,
					Type:        chartvariable.File,
					Description: "set the var from bundle default, so source path is bundle directory",
					Default:     "test.cert",
				},
			},
			requireNoErr: true,
			expected:     fmt.Sprintf("%s=%s", path, fmt.Sprintf("%stest.cert", relativePath)),
		},
		{
			name: "file not found",
			Bundle: newTestBundle(
				nil,
				nil,
				nil,
				"",
				fmt.Sprintf("%s/uds-bundle-helm-overrides-amd64-0.0.1.tar.zst", relativePath),
			),
			bundleVars: &[]types.BundleChartVariable{
				{
					Name:        varName,
					Path:        path,
					Type:        chartvariable.File,
					Description: "set the var from bundle default, so source path is bundle directory",
					Default:     "not-there-test.cert",
				},
			},
			requireNoErr: false,
			expected:     "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv("UDS_CERT")
			if tc.loadEnv {
				os.Setenv("UDS_CERT", fmt.Sprintf("%s/test.cert", relativePath))
			}

			overrideMap := map[string]map[string]*values.Options{componentName: {chartName: {}}}
			_, overrideData := tc.Bundle.loadVariables(types.Package{Name: pkgName}, nil)
			err := tc.Bundle.processOverrideVariables(overrideMap[componentName][chartName], *tc.bundleVars, overrideData)

			if tc.requireNoErr {
				require.NoError(t, err)
				require.Equal(t, tc.expected, overrideMap[componentName][chartName].FileValues[0])
			} else {
				require.Contains(t, err.Error(), "unable to find")
			}
		})
	}
}

func TestFormPkgViews(t *testing.T) {
	// types for readability in type assertions eg foo.(anyArr)[0].(viewOverVars)[bar]
	type anyArr = []interface{}
	type viewOverVars = map[string]map[string]interface{}

	const (
		componentName = "test-component"
		chartName     = "test-chart"
		pkgName       = "test-package"
	)

	type TestCase struct {
		name          string
		Bundle        Bundle
		loadEnv       bool
		expectedChart string
		expectedKey   string
		expectedVal   string
		envKey        string
		envVal        string
		bundleVars    types.BundleChartVariable
	}

	testCases := []TestCase{
		{
			name: "simple path, set by config",
			Bundle: newTestBundle(
				ConfigVariables{
					pkgName: {
						"VAR1": "set-by-config",
					},
				},
				nil,
				nil,
				"uds-config.yaml",
				"",
			),
			bundleVars:  types.BundleChartVariable{Name: "VAR1", Path: "path"},
			expectedKey: "VAR1",
			expectedVal: "set-by-config",
		},
		{
			name:   "complex path, set by bundle",
			Bundle: newTestBundle(nil, nil, nil, "", ""),
			bundleVars: types.BundleChartVariable{
				Name:    "VAR1",
				Path:    "a.complex.path",
				Default: "set-by-bundle",
			},
			expectedKey: "VAR1",
			expectedVal: "set-by-bundle",
		},
		{
			name:        "mask env var",
			Bundle:      newTestBundle(nil, nil, nil, "", ""),
			bundleVars:  types.BundleChartVariable{Name: "VAR1", Path: "path"},
			loadEnv:     true,
			envKey:      "UDS_VAR1",
			envVal:      "gets-masked",
			expectedKey: "VAR1",
			expectedVal: hiddenVar,
		},
		{
			name: "mask sensitive config var",
			Bundle: newTestBundle(
				ConfigVariables{
					pkgName: {
						"VAR1": "iamsensitive",
					},
				},
				nil,
				nil,
				"uds-config.yaml",
				"",
			),
			bundleVars: types.BundleChartVariable{
				Name:      "VAR1",
				Path:      "path",
				Sensitive: true,
			},
			expectedKey: "VAR1",
			expectedVal: hiddenVar,
		},
		{
			name: "mask file var",
			Bundle: newTestBundle(
				ConfigVariables{
					pkgName: {
						"VAR1": "../../test/bundles/07-helm-overrides/variable-files/test.cert",
					},
				},
				nil,
				nil,
				"uds-config.yaml",
				"",
			),
			bundleVars: types.BundleChartVariable{
				Name: "VAR1",
				Path: "path",
				Type: "file",
			},
			expectedKey: "VAR1",
			expectedVal: hiddenVar,
		},
		{
			name: "ensure multiple charts under same component are handled",
			Bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						Config: "uds-config.yaml",
						Variables: map[string]map[string]interface{}{
							pkgName: {
								"VAR1": "from-first-chart",
								"VAR2": "from-second-chart",
							},
						},
					},
				},
				bundle: types.UDSBundle{
					Packages: []types.Package{
						{
							Name: pkgName,
							Overrides: map[string]map[string]types.BundleChartOverrides{componentName: {
								chartName: {Variables: []types.BundleChartVariable{
									{
										Name: "VAR1",
										Path: "path",
									},
								}},
								"second-chart": {Variables: []types.BundleChartVariable{
									{
										Name: "VAR2",
										Path: "path",
									},
								}},
							}},
						},
					},
				},
			},
			expectedChart: "second-chart",
			expectedKey:   "VAR2",
			expectedVal:   "from-second-chart",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.loadEnv {
				os.Setenv(tc.envKey, tc.envVal)
				defer os.Unsetenv(tc.envKey)
			}
			if tc.expectedChart == "" {
				tc.expectedChart = chartName
			}

			if tc.bundleVars.Name != "" {
				tc.Bundle.bundle = types.UDSBundle{Packages: []types.Package{newTestPkg(pkgName, componentName, chartName, tc.bundleVars)}}
			}

			pkgViews := formPkgViews(&tc.Bundle)
			v, ok := pkgViews[0].overrides["overrides"].(anyArr)[0].(viewOverVars)[tc.expectedChart]["variables"]

			// check if the second chart is being used -- Go maps don't have strict ordering so value could be in 0 index or 1 index
			if !ok && len(pkgViews[0].overrides["overrides"].(anyArr)) > 1 {
				v = pkgViews[0].overrides["overrides"].(anyArr)[1].(viewOverVars)[tc.expectedChart]["variables"]
			}

			require.Contains(t, v.(map[string]interface{})[tc.expectedKey], tc.expectedVal)
		})
	}

	zarfVarTests := []TestCase{
		{
			name: "show zarf var",
			Bundle: newTestBundle(
				ConfigVariables{
					pkgName: {
						"VAR1": "zarf-var-set-by-config",
					},
				},
				nil,
				nil,
				"uds-config.yaml",
				"",
			),
			expectedKey: "VAR1",
			expectedVal: "zarf-var-set-by-config",
		},
		{
			name:        "hide zarf var with env var",
			loadEnv:     true,
			envKey:      "UDS_FOO",
			envVal:      "zarf-var-set-by-env",
			Bundle:      newTestBundle(nil, nil, nil, "", ""),
			expectedVal: hiddenVar,
			expectedKey: "FOO",
		},
	}

	for _, zarfVarTest := range zarfVarTests {
		t.Run(zarfVarTest.name, func(t *testing.T) {
			if zarfVarTest.loadEnv {
				os.Setenv(zarfVarTest.envKey, zarfVarTest.envVal)
				defer os.Unsetenv(zarfVarTest.envKey)
			}

			zarfVarTest.Bundle.bundle = types.UDSBundle{Packages: []types.Package{{Name: pkgName}}}
			pkgViews := formPkgViews(&zarfVarTest.Bundle)
			actualView := pkgViews[0].overrides["overrides"].(anyArr)[0]
			require.Contains(t, actualView.(map[string]interface{})[zarfVarTest.expectedKey], zarfVarTest.expectedVal)
		})
	}

	nilCheckTests := []TestCase{
		{
			name:       "ensure nil when override doesn't have a default and is not set",
			Bundle:     newTestBundle(nil, nil, nil, "", ""),
			bundleVars: types.BundleChartVariable{Name: "VAR1", Path: "path"},
		},
		{
			name:   "ensure nil when there are no overrides",
			Bundle: newTestBundle(nil, nil, nil, "", ""),
		},
	}

	for _, tc := range nilCheckTests {
		t.Run(tc.name, func(t *testing.T) {
			tc.Bundle.bundle = types.UDSBundle{Packages: []types.Package{newTestPkg(pkgName, componentName, chartName, tc.bundleVars)}}

			pkgViews := formPkgViews(&tc.Bundle)
			v := pkgViews[0].overrides["overrides"]
			require.Equal(t, 0, len(v.(anyArr)))
		})
	}
}

func TestFilterOverrides(t *testing.T) {
	chartVars := []types.BundleChartVariable{{Name: "over1"}, {Name: "over2"}}
	pkgVars := map[string]overrideData{"OVER1": {"val", valuesources.Config}, "ZARFVAR": {"val", valuesources.Env}}
	removeOverrides(pkgVars, chartVars)
	filtered := pkgVars
	actual := map[string]overrideData{"ZARFVAR": {"val", valuesources.Env}}
	require.Equal(t, actual, filtered)
}

func Test_newGitServerInfo(t *testing.T) {
	t.Parallel()
	pkgVars := zarfVarData{
		"INIT_REGISTRY_URL":           "any",
		"INIT_REGISTRY_PUSH_USERNAME": "any",
		"INIT_REGISTRY_PUSH_PASSWORD": "any",
		"INIT_REGISTRY_PULL_USERNAME": "any",
		"INIT_REGISTRY_PULL_PASSWORD": "any",
		"INIT_REGISTRY_SECRET":        "any",
		"INIT_REGISTRY_NODEPORT":      "any",
		"INIT_GIT_URL":                "fake.git",
		"INIT_GIT_PUSH_USERNAME":      "push-user",
		"INIT_GIT_PUSH_PASSWORD":      "push-secret!",
		"INIT_GIT_PULL_USERNAME":      "pull-user",
		"INIT_GIT_PULL_PASSWORD":      "pull-secret!",
		"INIT_ARTIFACT_URL":           "any",
		"INIT_ARTIFACT_PUSH_USERNAME": "any",
		"INIT_ARTIFACT_PUSH_TOKEN":    "any",
		"INIT_STORAGE_CLASS":          "any",
	}
	tests := []struct {
		name     string
		pkgKind  v1alpha1.ZarfPackageKind
		expected state.GitServerInfo
	}{
		{
			name:    "init config kind returns git server info",
			pkgKind: v1alpha1.ZarfInitConfig,
			expected: state.GitServerInfo{
				Address:      "fake.git",
				PushUsername: "push-user",
				PushPassword: "push-secret!",
				PullUsername: "pull-user",
				PullPassword: "pull-secret!",
			},
		},
		{
			name:     "package config kind returns empty git server info",
			pkgKind:  v1alpha1.ZarfPackageConfig,
			expected: state.GitServerInfo{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := newGitServerInfo(pkgVars, tt.pkgKind)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func Test_newRegistryInfo(t *testing.T) {
	t.Parallel()
	pkgVars := zarfVarData{
		"INIT_REGISTRY_URL":           "fake.io",
		"INIT_REGISTRY_PUSH_USERNAME": "push-user",
		"INIT_REGISTRY_PUSH_PASSWORD": "push-secret!",
		"INIT_REGISTRY_PULL_USERNAME": "pull-user",
		"INIT_REGISTRY_PULL_PASSWORD": "pull-secret!",
		"INIT_REGISTRY_SECRET":        "registry-secret",
		"INIT_REGISTRY_NODEPORT":      "1234",
		"INIT_GIT_URL":                "any",
		"INIT_GIT_PUSH_USERNAME":      "any",
		"INIT_GIT_PUSH_PASSWORD":      "any",
		"INIT_GIT_PULL_USERNAME":      "any",
		"INIT_GIT_PULL_PASSWORD":      "any",
		"INIT_ARTIFACT_URL":           "any",
		"INIT_ARTIFACT_PUSH_USERNAME": "any",
		"INIT_ARTIFACT_PUSH_TOKEN":    "any",
		"INIT_STORAGE_CLASS":          "any",
	}
	tests := []struct {
		name     string
		pkgKind  v1alpha1.ZarfPackageKind
		expected state.RegistryInfo
	}{
		{
			name:    "init config kind returns registry info",
			pkgKind: v1alpha1.ZarfInitConfig,
			expected: state.RegistryInfo{
				Address:      "fake.io",
				PushUsername: "push-user",
				PushPassword: "push-secret!",
				PullUsername: "pull-user",
				PullPassword: "pull-secret!",
				Secret:       "registry-secret",
				NodePort:     1234,
			},
		},
		{
			name:     "package config kind returns empty registry info",
			pkgKind:  v1alpha1.ZarfPackageConfig,
			expected: state.RegistryInfo{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := newRegistryInfo(pkgVars, tt.pkgKind)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func Test_newArtifactServerInfo(t *testing.T) {
	t.Parallel()
	pkgVars := zarfVarData{
		"INIT_REGISTRY_URL":           "any",
		"INIT_REGISTRY_PUSH_USERNAME": "any",
		"INIT_REGISTRY_PUSH_PASSWORD": "any",
		"INIT_REGISTRY_PULL_USERNAME": "any",
		"INIT_REGISTRY_PULL_PASSWORD": "any",
		"INIT_REGISTRY_SECRET":        "any",
		"INIT_REGISTRY_NODEPORT":      "any",
		"INIT_GIT_URL":                "any",
		"INIT_GIT_PUSH_USERNAME":      "any",
		"INIT_GIT_PUSH_PASSWORD":      "any",
		"INIT_GIT_PULL_USERNAME":      "any",
		"INIT_GIT_PULL_PASSWORD":      "any",
		"INIT_ARTIFACT_URL":           "fake.artifact",
		"INIT_ARTIFACT_PUSH_USERNAME": "push-user",
		"INIT_ARTIFACT_PUSH_TOKEN":    "push-token!",
		"INIT_STORAGE_CLASS":          "any",
	}
	tests := []struct {
		name     string
		pkgKind  v1alpha1.ZarfPackageKind
		expected state.ArtifactServerInfo
	}{
		{
			name:    "init config kind returns artifact info",
			pkgKind: v1alpha1.ZarfInitConfig,
			expected: state.ArtifactServerInfo{
				Address:      "fake.artifact",
				PushUsername: "push-user",
				PushToken:    "push-token!",
			},
		},
		{
			name:     "package config kind returns empty artifact info",
			pkgKind:  v1alpha1.ZarfPackageConfig,
			expected: state.ArtifactServerInfo{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := newArtifactServerInfo(pkgVars, tt.pkgKind)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func Test_newStorageClass(t *testing.T) {
	t.Parallel()
	pkgVars := zarfVarData{
		"INIT_REGISTRY_URL":           "any",
		"INIT_REGISTRY_PUSH_USERNAME": "any",
		"INIT_REGISTRY_PUSH_PASSWORD": "any",
		"INIT_REGISTRY_PULL_USERNAME": "any",
		"INIT_REGISTRY_PULL_PASSWORD": "any",
		"INIT_REGISTRY_SECRET":        "any",
		"INIT_REGISTRY_NODEPORT":      "any",
		"INIT_GIT_URL":                "any",
		"INIT_GIT_PUSH_USERNAME":      "any",
		"INIT_GIT_PUSH_PASSWORD":      "any",
		"INIT_GIT_PULL_USERNAME":      "any",
		"INIT_GIT_PULL_PASSWORD":      "any",
		"INIT_ARTIFACT_URL":           "any",
		"INIT_ARTIFACT_PUSH_USERNAME": "any",
		"INIT_ARTIFACT_PUSH_TOKEN":    "any",
		"INIT_STORAGE_CLASS":          "ebs",
	}
	tests := []struct {
		name     string
		pkgKind  v1alpha1.ZarfPackageKind
		expected string
	}{
		{
			name:     "init config kind returns storage class",
			pkgKind:  v1alpha1.ZarfInitConfig,
			expected: "ebs",
		},
		{
			name:     "package config kind returns empty storage class",
			pkgKind:  v1alpha1.ZarfPackageConfig,
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := newStorageClass(pkgVars, tt.pkgKind)
			require.Equal(t, tt.expected, actual)
		})
	}
}
