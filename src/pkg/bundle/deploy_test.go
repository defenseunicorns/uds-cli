package bundle

import (
	"os"
	"reflect"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
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
			// Set for select test cases to test precedence of env vars
			os.Unsetenv("UDS_FOO")
			if tc.loadEnvVar {
				os.Setenv("UDS_FOO", "set using env var")
			}
			actualPkgVars := tc.bundle.loadVariables(tc.pkg, tc.bundleExportVars)

			if !reflect.DeepEqual(actualPkgVars, tc.expectedPkgVars) {
				t.Errorf("Test case %s failed. Expected %v, got %v", tc.name, tc.expectedPkgVars, actualPkgVars)
			}
		})
	}
}
