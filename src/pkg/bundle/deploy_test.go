// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

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
	"helm.sh/helm/v3/pkg/cli/values"
)

func TestLoadVariablesPrecedence(t *testing.T) {
	testCases := []struct {
		name             string
		description      string
		pkg              types.Package
		bundle           Bundle
		bundleExportVars map[string]map[string]string
		loadEnvVar       bool
		expectedPkgVars  map[string]string
	}{
		{
			name:       "--set flag precedence",
			loadEnvVar: true,
			pkg: types.Package{
				Name: "fooPkg",
				Imports: []types.BundleVariableImport{
					{
						Name:    "foo",
						Package: "bazPkg",
					},
				},
			},
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						Variables: map[string]map[string]interface{}{
							"fooPkg": {
								"foo": "set from variables key in uds-config.yaml",
							},
						},
						// set from uds-config.yaml
						SharedVariables: map[string]interface{}{
							"foo": "set from shared key in uds-config.yaml",
						},
						SetVariables: map[string]string{
							"foo": "set using --set flag",
						},
					},
				},
			},
			bundleExportVars: map[string]map[string]string{
				"barPkg": {
					"foo": "exported from another pkg",
				},
				"bazPkg": {
					"foo": "imported from a specific pkg",
				},
			},
			expectedPkgVars: map[string]string{
				"FOO": "set using --set flag",
			},
		},
		{
			name:       "env var precedence",
			loadEnvVar: true,
			pkg: types.Package{
				Name: "fooPkg",
				Imports: []types.BundleVariableImport{
					{
						Name:    "foo",
						Package: "bazPkg",
					},
				},
			},
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						Variables: map[string]map[string]interface{}{
							"fooPkg": {
								"foo": "set from variables key in uds-config.yaml",
							},
						},
						// set from uds-config.yaml
						SharedVariables: map[string]interface{}{
							"foo": "set from shared key in uds-config.yaml",
						},
					},
				},
			},
			bundleExportVars: map[string]map[string]string{
				"barPkg": {
					"foo": "exported from another pkg",
				},
				"bazPkg": {
					"foo": "imported from a specific pkg",
				},
			},
			expectedPkgVars: map[string]string{
				"FOO": "set using env var",
			},
		},
		{
			name: "uds-config variables key precedence",
			pkg: types.Package{
				Name: "fooPkg",
				Imports: []types.BundleVariableImport{
					{
						Name:    "foo",
						Package: "bazPkg",
					},
				},
			},
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						Variables: map[string]map[string]interface{}{
							"fooPkg": {
								"foo": "set from variables key in uds-config.yaml",
							},
						},
						// set from uds-config.yaml
						SharedVariables: map[string]interface{}{
							"foo": "set from shared key in uds-config.yaml",
						},
					},
				},
			},
			bundleExportVars: map[string]map[string]string{
				"barPkg": {
					"foo": "exported from another pkg",
				},
				"bazPkg": {
					"foo": "imported from a specific pkg",
				},
			},
			expectedPkgVars: map[string]string{
				"FOO": "set from variables key in uds-config.yaml",
			},
		},
		{
			name: "uds-config shared key precedence",
			pkg: types.Package{
				Name: "fooPkg",
				Imports: []types.BundleVariableImport{
					{
						Name:    "foo",
						Package: "bazPkg",
					},
				},
			},
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						// set from uds-config.yaml
						SharedVariables: map[string]interface{}{
							"foo": "set from shared key in uds-config.yaml",
						},
					},
				},
			},
			bundleExportVars: map[string]map[string]string{
				"barPkg": {
					"foo": "exported from another pkg",
				},
				"bazPkg": {
					"foo": "imported from a specific pkg",
				},
			},
			expectedPkgVars: map[string]string{
				"FOO": "set from shared key in uds-config.yaml",
			},
		},
		{
			name: "uds-config shared key precedence",
			pkg: types.Package{
				Name: "fooPkg",
				Imports: []types.BundleVariableImport{
					{
						Name:    "foo",
						Package: "bazPkg",
					},
				},
			},
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						SharedVariables: nil,
					},
				},
			},
			bundleExportVars: map[string]map[string]string{
				"barPkg": {
					"foo": "exported from another pkg",
				},
				"bazPkg": {
					"foo": "imported from a specific pkg",
				},
			},
			expectedPkgVars: map[string]string{
				"FOO": "imported from a specific pkg",
			},
		},
		{
			name: "uds-config global export precedence",
			pkg: types.Package{
				Name: "fooPkg",
			},
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						SharedVariables: nil,
					},
				},
			},
			bundleExportVars: map[string]map[string]string{
				"barPkg": {
					"foo": "exported from another pkg",
				},
			},
			expectedPkgVars: map[string]string{
				"FOO": "exported from another pkg",
			},
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// unset arch var that gets applied automatically when doing 'uds run' so it doesn't get in the way
			os.Unsetenv("UDS_ARCH")

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

func TestHelmOverrideVariablePrecedence(t *testing.T) {
	// args for b.processOverrideVariables fn
	type args struct {
		pkgName       string
		variables     *[]types.BundleChartVariable
		componentName string
		chartName     string
	}
	testCases := []struct {
		name        string
		bundle      Bundle
		args        args
		loadEnvVar  bool
		expectedVal string
	}{
		{
			name:       "--set flag precedence",
			loadEnvVar: true,
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						SetVariables: map[string]string{
							"foo": "set using --set flag",
						},
						SharedVariables: map[string]interface{}{
							"FOO": "set from shared key in uds-config.yaml",
						},
						Variables: map[string]map[string]interface{}{
							"FOO": {
								"foo": "set from variables key in uds-config.yaml",
							},
						},
					},
				},
			},
			args: args{
				pkgName: "fooPkg",
				variables: &[]types.BundleChartVariable{
					{
						Name:    "foo",
						Default: "default value",
					},
				},
				componentName: "component",
				chartName:     "chart",
			},
			expectedVal: "=set using --set flag",
		},
		{
			name:       "env var precedence",
			loadEnvVar: true,
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						SharedVariables: map[string]interface{}{
							"FOO": "set from shared key in uds-config.yaml",
						},
						Variables: map[string]map[string]interface{}{
							"fooPkg": {
								"FOO": "set from variables key in uds-config.yaml",
							},
						},
					},
				},
			},
			args: args{
				pkgName: "fooPkg",
				variables: &[]types.BundleChartVariable{
					{
						Name:    "foo",
						Default: "default value",
					},
				},
				componentName: "component",
				chartName:     "chart",
			},
			expectedVal: "=set using env var",
		},
		{
			name: "uds-config variables key precedence",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						SharedVariables: map[string]interface{}{
							"FOO": "set from shared key in uds-config.yaml",
						},
						Variables: map[string]map[string]interface{}{
							"fooPkg": {
								"FOO": "set from variables key in uds-config.yaml",
							},
						},
					},
				},
			},
			args: args{
				pkgName: "fooPkg",
				variables: &[]types.BundleChartVariable{
					{
						Name:    "foo",
						Default: "default value",
					},
				},
				componentName: "component",
				chartName:     "chart",
			},
			expectedVal: "=set from variables key in uds-config.yaml",
		},
		{
			name: "uds-config shared key precedence",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						SharedVariables: map[string]interface{}{
							"FOO": "set from shared key in uds-config.yaml",
						},
					},
				},
			},
			args: args{
				pkgName: "fooPkg",
				variables: &[]types.BundleChartVariable{
					{
						Name:    "foo",
						Default: "default value",
					},
				},
				componentName: "component",
				chartName:     "chart",
			},
			expectedVal: "=set from shared key in uds-config.yaml",
		},
		{
			name: "use variable default",
			bundle: Bundle{
				cfg: &types.BundleConfig{},
			},
			args: args{
				pkgName: "fooPkg",
				variables: &[]types.BundleChartVariable{
					{
						Name:    "foo",
						Default: "default value",
					},
				},
				componentName: "component",
				chartName:     "chart",
			},
			expectedVal: "=default value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := &Bundle{
				cfg:    tc.bundle.cfg,
				bundle: tc.bundle.bundle,
				tmp:    tc.bundle.tmp,
			}
			// Set for select test cases to test precedence of env vars
			os.Unsetenv("UDS_FOO")
			if tc.loadEnvVar {
				os.Setenv("UDS_FOO", "set using env var")
			}
			overrideMap := map[string]map[string]*values.Options{tc.args.componentName: {tc.args.chartName: {}}}
			_, overrideData := b.loadVariables(types.Package{Name: tc.args.pkgName}, nil)
			err := b.processOverrideVariables(overrideMap[tc.args.componentName][tc.args.chartName], *tc.args.variables, overrideData)
			require.NoError(t, err)
			if tc.expectedVal == "" {
				require.Equal(t, 0, len(overrideMap))
			}
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

	type args struct {
		pkgName       string
		variables     *[]types.BundleChartVariable
		componentName string
		chartName     string
	}
	testCases := []struct {
		name         string
		bundle       Bundle
		args         args
		loadEnv      bool
		requireNoErr bool
		expected     string
	}{
		{
			name: "with --set",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						SetVariables: map[string]string{
							varName: fmt.Sprintf("%s/test.cert", relativePath),
						},
					},
				},
			},
			args: args{
				pkgName: pkgName,
				variables: &[]types.BundleChartVariable{
					{
						Name:        varName,
						Path:        path,
						Type:        chartvariable.File,
						Description: "set the var from cli, so source path is current working directory (eg. /home/user/repos/uds-cli/...)",
					},
				},
				componentName: componentName,
				chartName:     chartName,
			},
			requireNoErr: true,
			expected:     fmt.Sprintf("%s=%s", path, filepath.Join(cwd, fmt.Sprintf("%s/test.cert", relativePath))),
		},
		{
			name: "with UDS_VAR",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{},
				},
			},
			args: args{
				pkgName: pkgName,
				variables: &[]types.BundleChartVariable{
					{
						Name:        varName,
						Path:        path,
						Type:        chartvariable.File,
						Description: "set the var from env, so source path is current working directory (eg. /home/user/repos/uds-cli/...)",
					},
				},
				componentName: componentName,
				chartName:     chartName,
			},
			loadEnv:      true,
			requireNoErr: true,
			expected:     fmt.Sprintf("%s=%s", path, filepath.Join(cwd, fmt.Sprintf("%s/test.cert", relativePath))),
		},
		{
			name: "with Config",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						Config: fmt.Sprintf("%s/uds-config.yaml", relativePath),
						Variables: map[string]map[string]interface{}{
							pkgName: {
								varName: "test.cert",
							},
						},
					},
				},
			},
			args: args{
				pkgName: pkgName,
				variables: &[]types.BundleChartVariable{
					{
						Name:        varName,
						Path:        path,
						Type:        chartvariable.File,
						Description: "set the var from config, so source path is config directory",
					},
				},
				componentName: componentName,
				chartName:     chartName,
			},
			requireNoErr: true,
			expected:     fmt.Sprintf("%s=%s", path, fmt.Sprintf("%stest.cert", relativePath)),
		},
		{
			name: "with Bundle",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						Source: fmt.Sprintf("%s/uds-bundle-helm-overrides-amd64-0.0.1.tar.zst", relativePath),
					},
				},
			},
			args: args{
				pkgName: pkgName,
				variables: &[]types.BundleChartVariable{
					{
						Name:        varName,
						Path:        path,
						Type:        chartvariable.File,
						Description: "set the var from bundle default, so source path is bundle directory",
						Default:     "test.cert",
					},
				},
				componentName: componentName,
				chartName:     chartName,
			},
			requireNoErr: true,
			expected:     fmt.Sprintf("%s=%s", path, fmt.Sprintf("%stest.cert", relativePath)),
		},
		{
			name: "file not found",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						Source: fmt.Sprintf("%s/uds-bundle-helm-overrides-amd64-0.0.1.tar.zst", relativePath),
					},
				},
			},
			args: args{
				pkgName: pkgName,
				variables: &[]types.BundleChartVariable{
					{
						Name:        varName,
						Path:        path,
						Type:        chartvariable.File,
						Description: "set the var from bundle default, so source path is bundle directory",
						Default:     "not-there-test.cert",
					},
				},
				componentName: componentName,
				chartName:     chartName,
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

			overrideMap := map[string]map[string]*values.Options{tc.args.componentName: {tc.args.chartName: {}}}
			_, overrideData := tc.bundle.loadVariables(types.Package{Name: tc.args.pkgName}, nil)
			err := tc.bundle.processOverrideVariables(overrideMap[tc.args.componentName][tc.args.chartName], *tc.args.variables, overrideData)

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
	const (
		componentName     = "test-component"
		chartName         = "test-chart"
		pkgName           = "test-package"
		optionalComponent = "test-optional-component"
	)

	type TestCase struct {
		name          string
		bundle        Bundle
		loadEnv       bool
		expectedIndex int
		expectedChart string
		expectedKey   string
		expectedVal   string
		envKey        string
		envVal        string
	}

	setUpPkg := func(overVar types.BundleChartVariable) types.Package {
		return types.Package{Name: pkgName,
			OptionalComponents: []string{optionalComponent},
			Overrides: map[string]map[string]types.BundleChartOverrides{
				componentName: {
					chartName: {
						Variables: []types.BundleChartVariable{overVar},
					},
				},
			},
		}
	}

	testCases := []TestCase{
		{
			name: "simple path, set by config",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						Config: "uds-config.yaml",
						Variables: map[string]map[string]interface{}{
							pkgName: {
								"VAR1": "set-by-config",
							},
						},
					},
				},
				bundle: types.UDSBundle{
					Packages: []types.Package{setUpPkg(types.BundleChartVariable{Name: "VAR1", Path: "path"})},
				},
			},
			expectedKey: "VAR1",
			expectedVal: "set-by-config",
		},
		{
			name: "complex path, set by bundle",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{},
				},
				bundle: types.UDSBundle{
					Packages: []types.Package{setUpPkg(types.BundleChartVariable{
						Name:    "VAR1",
						Path:    "a.complex.path",
						Default: "set-by-bundle",
					})},
				},
			},
			expectedKey: "VAR1",
			expectedVal: "set-by-bundle",
		},
		{
			name: "mask env var",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{},
				},
				bundle: types.UDSBundle{
					Packages: []types.Package{setUpPkg(types.BundleChartVariable{Name: "VAR1", Path: "path"})},
				},
			},
			loadEnv:     true,
			envKey:      "UDS_VAR1",
			envVal:      "gets-masked",
			expectedKey: "VAR1",
			expectedVal: hiddenVar,
		},
		{
			name: "mask file var",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						Config: "uds-config.yaml",
						Variables: map[string]map[string]interface{}{
							pkgName: {
								"VAR1": "../../test/bundles/07-helm-overrides/variable-files/test.cert",
							},
						},
					},
				},
				bundle: types.UDSBundle{
					Packages: []types.Package{setUpPkg(types.BundleChartVariable{
						Name: "VAR1",
						Path: "path",
						Type: "file",
					})},
				},
			},
			expectedKey: "VAR1",
			expectedVal: hiddenVar,
		},
		{
			name: "ensure multiple charts under same component are handled",
			bundle: Bundle{
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
			expectedIndex: 1,
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
			fmt.Println(tc.expectedChart)
			pkgViews := formPkgViews(&tc.bundle)
			v := pkgViews[0].overrides["overrides"].([]interface{})[tc.expectedIndex].(map[string]map[string]interface{})[tc.expectedChart]["variables"]
			fmt.Println(v)
			require.Contains(t, v.(map[string]interface{})[tc.expectedKey], tc.expectedVal)

			// ensure that optionalComponents are part of the view when included in a bundle's pkg
			if len(pkgViews[0].optionalComponents) > 0 {
				require.Contains(t, pkgViews[0].optionalComponents[0], optionalComponent)
			}
		})
	}

	zarfVarTests := []TestCase{
		{
			name: "show zarf var",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{
						Config: "uds-config.yaml",
						Variables: map[string]map[string]interface{}{
							pkgName: {
								"VAR1": "zarf-var-set-by-config",
							},
						},
					},
				},
				bundle: types.UDSBundle{Packages: []types.Package{{Name: pkgName}}},
			},
			expectedKey: "VAR1",
			expectedVal: "zarf-var-set-by-config",
		},
		{
			name:    "hide zarf var with env var",
			loadEnv: true,
			envKey:  "UDS_FOO",
			envVal:  "zarf-var-set-by-env",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{},
				},
				bundle: types.UDSBundle{Packages: []types.Package{{Name: pkgName}}},
			},
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
			pkgViews := formPkgViews(&zarfVarTest.bundle)
			actualView := pkgViews[0].overrides["overrides"].([]interface{})[0]
			require.Contains(t, actualView.(map[string]interface{})[zarfVarTest.expectedKey], zarfVarTest.expectedVal)
		})
	}

	nilCheckTests := []TestCase{
		{
			name: "ensure nil when override doesn't have a default and is not set",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{},
				},
				bundle: types.UDSBundle{
					Packages: []types.Package{setUpPkg(types.BundleChartVariable{Name: "VAR1", Path: "path"})},
				},
			},
		},
		{
			name: "ensure nil when there are no overrides",
			bundle: Bundle{
				cfg: &types.BundleConfig{
					DeployOpts: types.BundleDeployOptions{},
				},
				bundle: types.UDSBundle{
					Packages: []types.Package{{Name: pkgName}},
				},
			},
		},
	}

	for _, tc := range nilCheckTests {
		t.Run(tc.name, func(t *testing.T) {
			pkgViews := formPkgViews(&tc.bundle)

			v := pkgViews[0].overrides["overrides"]

			require.Equal(t, 0, len(v.([]interface{})))
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
