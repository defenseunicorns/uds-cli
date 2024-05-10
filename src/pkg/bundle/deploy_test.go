package bundle

import (
	"os"
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
