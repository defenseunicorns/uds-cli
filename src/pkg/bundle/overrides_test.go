// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
)

func TestLoadChartOverridesPreservesTicketCredentialValues(t *testing.T) {
	for _, test := range []struct {
		name       string
		credential string
	}{
		{name: "plain text", credential: "example credential"},
		{name: "quote", credential: `example"credential`},
		{name: "backslash q", credential: `example\q`},
		{name: "backslash one", credential: `example\1`},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings := map[string]interface{}{
				"notifications": map[string]interface{}{"enabled": true},
				"externalService": map[string]interface{}{
					"endpoint":   "https://example.invalid",
					"credential": "${EXTERNAL_SERVICE_CREDENTIAL}",
				},
			}
			credentials := []interface{}{map[string]interface{}{"credential": "${EXTERNAL_SERVICE_CREDENTIAL}"}}
			pkg := types.Package{
				Overrides: map[string]map[string]types.BundleChartOverrides{"example component": {"example chart": {
					Values: []types.BundleChartValue{
						{Path: "application.settings", Value: settings},
						{Path: "application.credentials", Value: credentials},
					},
				}}},
			}

			overrides, _, err := (&Bundle{}).loadChartOverrides(pkg, bOverridesData{
				"EXTERNAL_SERVICE_CREDENTIAL": {value: test.credential},
			})

			require.NoError(t, err)
			require.Equal(t, map[string]interface{}{
				"application": map[string]interface{}{
					"settings": map[string]interface{}{
						"notifications": map[string]interface{}{"enabled": true},
						"externalService": map[string]interface{}{
							"endpoint":   "https://example.invalid",
							"credential": test.credential,
						},
					},
					"credentials": []interface{}{map[string]interface{}{"credential": test.credential}},
				},
			}, overrides["example component"]["example chart"])
			require.Equal(t, "${EXTERNAL_SERVICE_CREDENTIAL}", settings["externalService"].(map[string]interface{})["credential"])
			require.Equal(t, "${EXTERNAL_SERVICE_CREDENTIAL}", credentials[0].(map[string]interface{})["credential"])
		})
	}
}

func TestLoadChartOverridesTemplatedMapKeys(t *testing.T) {
	t.Run("preserves quotes and backslashes", func(t *testing.T) {
		value := map[string]interface{}{"${KEY}": "value"}
		pkg := types.Package{
			Overrides: map[string]map[string]types.BundleChartOverrides{"component": {"chart": {
				Values: []types.BundleChartValue{{Path: "object", Value: value}},
			}}},
		}

		overrides, _, err := (&Bundle{}).loadChartOverrides(pkg, bOverridesData{"KEY": {value: `resolved"\key`}})

		require.NoError(t, err)
		require.Equal(t, map[string]interface{}{`resolved"\key`: "value"}, overrides["component"]["chart"]["object"])
		require.Equal(t, "value", value["${KEY}"])
	})

	t.Run("rejects collisions", func(t *testing.T) {
		pkg := types.Package{
			Name: "example package",
			Overrides: map[string]map[string]types.BundleChartOverrides{"component": {"chart": {
				Values: []types.BundleChartValue{{
					Path:  "object",
					Value: map[string]interface{}{"${KEY}": "one", "duplicate": "two"},
				}},
			}}},
		}

		_, _, err := (&Bundle{}).loadChartOverrides(pkg, bOverridesData{"KEY": {value: "duplicate"}})

		require.ErrorContains(t, err, "templated map keys resolve to the same value")
		require.ErrorContains(t, err, `package "example package" component "component" chart "chart"`)
		require.ErrorContains(t, err, `path "object"`)
	})
}

func TestFormPkgViewsLeavesUnavailableImportedTemplatedKeysUnresolved(t *testing.T) {
	bundle := newTestBundle(nil, nil, nil, "", "")
	bundle.bundle = types.UDSBundle{Packages: []types.Package{{
		Name: "example package",
		Imports: []types.BundleVariableImport{
			{Name: "FIRST", Package: "first package"},
			{Name: "SECOND", Package: "second package"},
		},
		Overrides: map[string]map[string]types.BundleChartOverrides{"component": {"chart": {
			Values: []types.BundleChartValue{{
				Path:  "object",
				Value: map[string]interface{}{"${FIRST}": "one", "${SECOND}": "two"},
			}},
		}}},
	}}}

	_, err := formPkgViewsWithMetadata(&bundle, func(types.Package) (v1alpha1.ZarfPackage, error) {
		return v1alpha1.ZarfPackage{}, nil
	})

	require.NoError(t, err)
}

func TestFormPkgViewsPropagatesTicketOverrideErrorWithoutCredential(t *testing.T) {
	const credential = `example"credential\q`
	t.Setenv("UDS_EXTERNAL_SERVICE_CREDENTIAL", credential)
	bundle := newTestBundle(nil, nil, nil, "", "")
	bundle.bundle = types.UDSBundle{Packages: []types.Package{{
		Name: "example package",
		Overrides: map[string]map[string]types.BundleChartOverrides{"example component": {"example chart": {
			Values: []types.BundleChartValue{{
				Path: "application.settings[-1]",
				Value: map[string]interface{}{
					"notifications": map[string]interface{}{"enabled": true},
					"externalService": map[string]interface{}{
						"endpoint":   "https://example.invalid",
						"credential": "${EXTERNAL_SERVICE_CREDENTIAL}",
					},
				},
			}},
		}}},
	}}}

	views, err := formPkgViewsWithMetadata(&bundle, func(types.Package) (v1alpha1.ZarfPackage, error) {
		return v1alpha1.ZarfPackage{}, nil
	})

	require.Nil(t, views)
	require.ErrorContains(t, err, "unable to process Helm values overrides")
	require.ErrorContains(t, err, "example package")
	require.NotContains(t, err.Error(), credential)
}
