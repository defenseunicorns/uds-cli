package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
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
			actualPkgVars := tc.bundle.loadVariables(tc.pkg, tc.bundleExportVars)
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
		{
			name: "no variable overrides",
			bundle: Bundle{
				cfg: &types.BundleConfig{},
			},
			args: args{
				pkgName: "fooPkg",
				variables: &[]types.BundleChartVariable{
					{
						Name: "foo",
					},
				},
				componentName: "component",
				chartName:     "chart",
			},
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
			overrideMap := map[string]map[string]*values.Options{}
			err := b.processOverrideVariables(&overrideMap, tc.args.pkgName, tc.args.variables, tc.args.componentName, tc.args.chartName)
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
						Type:        types.File,
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
						Type:        types.File,
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
						Type:        types.File,
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
						Type:        types.File,
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
						Type:        types.File,
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

			overrideMap := map[string]map[string]*values.Options{}
			err := tc.bundle.processOverrideVariables(&overrideMap, tc.args.pkgName, tc.args.variables, tc.args.componentName, tc.args.chartName)

			if tc.requireNoErr {
				require.NoError(t, err)
				require.Equal(t, tc.expected, overrideMap[componentName][chartName].FileValues[0])
			} else {
				require.Contains(t, err.Error(), "unable to find")
			}
		})
	}
}
