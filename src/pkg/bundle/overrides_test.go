// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/uds-cli/src/types/chartvariable"
	"github.com/stretchr/testify/require"
)

func TestBundleOverrideVariableMigration(t *testing.T) {
	for _, test := range []struct {
		name       string
		credential string
		typeOf     chartvariable.Type
	}{
		{name: "raw variable", credential: `example"credential`, typeOf: chartvariable.Raw},
		{name: "file variable", credential: `example"credential\q\1`, typeOf: chartvariable.File},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := test.credential
			if test.typeOf == chartvariable.File {
				input = filepath.Join(t.TempDir(), "credential")
				require.NoError(t, os.WriteFile(input, []byte(test.credential), 0o600))
			}
			t.Setenv("UDS_EXTERNAL_SERVICE_CREDENTIAL", input)
			bundle := newTestBundle(nil, nil, nil, "", "")
			pkg := types.Package{
				Name: "example package",
				Overrides: map[string]map[string]types.BundleChartOverrides{
					"example component": {
						"example chart": {
							Values: []types.BundleChartValue{{
								Path: "application.settings",
								Value: map[string]interface{}{
									"notifications":   map[string]interface{}{"enabled": true},
									"externalService": map[string]interface{}{"endpoint": "https://example.invalid"},
								},
							}},
							Variables: []types.BundleChartVariable{{
								Name:      "EXTERNAL_SERVICE_CREDENTIAL",
								Path:      "application.settings.externalService.credential",
								Type:      test.typeOf,
								Sensitive: true,
							}},
						},
					},
				},
			}

			_, variableData := bundle.loadVariables(pkg, nil)
			overrides, _, err := bundle.loadChartOverrides(pkg, variableData)

			require.NoError(t, err)
			require.Equal(t, test.credential, overrides["example component"]["example chart"]["application"].(map[string]interface{})["settings"].(map[string]interface{})["externalService"].(map[string]interface{})["credential"])
		})
	}
}
