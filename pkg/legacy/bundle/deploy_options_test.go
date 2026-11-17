// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
	"slices"
	"testing"

	"helm.sh/helm/v4/pkg/chart/common"
	chartutil "helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/engine"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/api/v1alpha1" //nolint:staticcheck // Verify Legacy v1alpha1 package compatibility.
	"github.com/zarf-dev/zarf/src/api/v1beta1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/state"
	"github.com/zarf-dev/zarf/src/pkg/value"
)

func TestConfigurePackageDeployVariables(t *testing.T) {
	t.Parallel()
	for _, version := range []string{v1alpha1.APIVersion, v1beta1.APIVersion} {
		t.Run(version, func(t *testing.T) {
			opts := packager.DeployOptions{SetVariables: map[string]string{"APP_SETTING": "test-value"}}
			err := configurePackageDeployOptions(&opts, api.Package{APIVersion: version, Kind: api.ZarfPackageConfig})
			require.NoError(t, err)
			if version == v1beta1.APIVersion {
				require.Empty(t, opts.SetVariables)
			} else {
				require.Equal(t, "test-value", opts.SetVariables["APP_SETTING"])
			}
		})
	}
}

func TestInitArtifactSettingsRejected(t *testing.T) {
	b := Bundle{cfg: &types.BundleConfig{}}
	for _, key := range []string{config.ArtifactURL, config.ArtifactPushUsername, config.ArtifactPushToken} {
		t.Run(key, func(t *testing.T) {
			_, err := b.newDeployOptions(t.Context(), types.Package{Name: "init"}, zarfVarData{key: "private-value"}, nil, api.ZarfInitConfig, state.RegistryInfo{})
			require.ErrorContains(t, err, key+" is unsupported")
			require.NotContains(t, err.Error(), "private-value")
		})
	}
}

func TestDeployConnectivity(t *testing.T) {
	original := config.Dev
	t.Cleanup(func() { config.Dev = original })
	b := Bundle{cfg: &types.BundleConfig{}}
	for _, connected := range []bool{false, true} {
		config.Dev = connected
		opts, err := b.newDeployOptions(t.Context(), types.Package{Name: "app"}, nil, nil, api.ZarfPackageConfig, state.RegistryInfo{})
		require.NoError(t, err)
		require.Equal(t, connected, opts.Connectivity.IsConnected())
	}
}

func TestInitStorageClassPreservesExplicitComponentVariables(t *testing.T) {
	t.Parallel()
	opts := packager.DeployOptions{SetVariables: map[string]string{
		config.StorageClass:        "default-init-storage",
		"REGISTRY_STORAGE_CLASS":   "registry-storage",
		"GIT_SERVER_STORAGE_CLASS": "",
	}}
	require.NoError(t, configurePackageDeployOptions(&opts, api.Package{APIVersion: v1alpha1.APIVersion, Kind: api.ZarfInitConfig}))
	require.Equal(t, "registry-storage", opts.SetVariables["REGISTRY_STORAGE_CLASS"])
	require.Empty(t, opts.SetVariables["GIT_SERVER_STORAGE_CLASS"])
}

func TestOldAlphaInitStorageClassRendersPVC(t *testing.T) {
	t.Parallel()
	for _, target := range []struct{ component, chart string }{
		{"zarf-seed-registry", "docker-registry"},
		{"zarf-registry", "docker-registry"},
		{"git-server", "gitea"},
	} {
		t.Run(target.component, func(t *testing.T) {
			pkg := api.Package{
				APIVersion: v1alpha1.APIVersion, Kind: api.ZarfInitConfig,
				Components: []api.Component{{Name: target.component, Charts: []api.Chart{{Name: target.chart}}}},
			}
			opts := packager.DeployOptions{SetVariables: map[string]string{config.StorageClass: "custom-storage"}}
			require.NoError(t, configurePackageDeployOptions(&opts, pkg))

			// Zarf 1.0 renders older init packages' ###ZARF_STORAGE_CLASS###
			// placeholder as an empty string before applying chart overrides.
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{APIVersion: "v2", Name: target.chart, Version: "1.0.0"},
				Values:   map[string]any{"persistence": map[string]any{"storageClass": ""}},
				Templates: []*common.File{{Name: "templates/pvc.yaml", Data: []byte(`apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: storage
spec:
{{- if .Values.persistence.storageClass }}
  storageClassName: {{ .Values.persistence.storageClass | quote }}
{{- end }}
`)}},
			}
			values, err := chartutil.ToRenderValues(helmChart, opts.ValuesOverridesMap[target.component][target.chart], common.ReleaseOptions{Name: "init", Namespace: "zarf"}, nil)
			require.NoError(t, err)
			rendered, err := engine.Render(helmChart, values)
			require.NoError(t, err)
			require.Contains(t, rendered[target.chart+"/templates/pvc.yaml"], `storageClassName: "custom-storage"`)
		})
	}
}

func TestAlphaInitStorageClassUsesDeclaredVariables(t *testing.T) {
	t.Parallel()
	for _, declared := range [][]string{
		{"REGISTRY_STORAGE_CLASS", "GIT_SERVER_STORAGE_CLASS"},
		{"REGISTRY_STORAGE_CLASS"},
		{"GIT_SERVER_STORAGE_CLASS"},
	} {
		t.Run(fmt.Sprint(declared), func(t *testing.T) {
			pkg := api.Package{
				APIVersion: v1alpha1.APIVersion, Kind: api.ZarfInitConfig,
				Components: []api.Component{
					{Name: "zarf-registry", Charts: []api.Chart{{Name: "docker-registry"}}},
					{Name: "git-server", Charts: []api.Chart{{Name: "gitea"}}},
				},
			}
			for _, name := range declared {
				pkg.Variables = append(pkg.Variables, api.InteractiveVariable{Variable: api.Variable{Name: name}}) //nolint:staticcheck // Model alpha init packages declaring the newer storage-class variables.
			}
			opts := packager.DeployOptions{SetVariables: map[string]string{config.StorageClass: "custom-storage"}}
			require.NoError(t, configurePackageDeployOptions(&opts, pkg))
			for _, target := range []struct{ component, chart, variable string }{
				{"zarf-registry", "docker-registry", "REGISTRY_STORAGE_CLASS"},
				{"git-server", "gitea", "GIT_SERVER_STORAGE_CLASS"},
			} {
				require.Equal(t, "custom-storage", opts.SetVariables[target.variable])
				if slices.Contains(declared, target.variable) {
					require.NotContains(t, opts.ValuesOverridesMap, target.component)
				} else {
					require.Equal(t, value.Values{"persistence": map[string]any{"storageClass": "custom-storage"}}, opts.ValuesOverridesMap[target.component][target.chart])
				}
			}
		})
	}
}

func TestOldAlphaInitStorageClassPreservesOverrides(t *testing.T) {
	t.Parallel()
	overrides := packager.ValuesOverrides{
		"zarf-registry": {"docker-registry": {
			"persistence": map[string]any{"size": "50Gi"}, "replicaCount": 2,
		}},
		"git-server": {"gitea": {
			"persistence": map[string]any{"storageClass": "chart-specific", "size": "20Gi"},
		}},
	}
	pkg := api.Package{
		APIVersion: v1alpha1.APIVersion, Kind: api.ZarfInitConfig,
		Components: []api.Component{
			{Name: "zarf-registry", Charts: []api.Chart{{Name: "docker-registry"}}},
			{Name: "git-server", Charts: []api.Chart{{Name: "gitea"}}},
		},
	}
	opts := packager.DeployOptions{
		SetVariables: map[string]string{config.StorageClass: "custom-storage"}, ValuesOverridesMap: overrides,
	}
	require.NoError(t, configurePackageDeployOptions(&opts, pkg))
	require.Equal(t, value.Values{
		"persistence": map[string]any{"storageClass": "custom-storage", "size": "50Gi"}, "replicaCount": 2,
	}, opts.ValuesOverridesMap["zarf-registry"]["docker-registry"])
	require.Equal(t, overrides["git-server"]["gitea"], opts.ValuesOverridesMap["git-server"]["gitea"])

	opts.ValuesOverridesMap["zarf-registry"]["docker-registry"]["persistence"].(map[string]any)["size"] = "changed"
	opts.ValuesOverridesMap["git-server"]["gitea"]["persistence"].(map[string]any)["storageClass"] = "changed"
	require.Equal(t, value.Values{"persistence": map[string]any{"size": "50Gi"}, "replicaCount": 2}, overrides["zarf-registry"]["docker-registry"])
	require.Equal(t, value.Values{"persistence": map[string]any{"storageClass": "chart-specific", "size": "20Gi"}}, overrides["git-server"]["gitea"])
}

func TestOldAlphaInitStorageClassUsesExplicitVariables(t *testing.T) {
	t.Parallel()
	pkg := api.Package{
		APIVersion: v1alpha1.APIVersion, Kind: api.ZarfInitConfig,
		Components: []api.Component{
			{Name: "zarf-registry", Charts: []api.Chart{{Name: "docker-registry"}}},
			{Name: "git-server", Charts: []api.Chart{{Name: "gitea"}}},
		},
	}
	opts := packager.DeployOptions{SetVariables: map[string]string{
		config.StorageClass: "custom-storage", "REGISTRY_STORAGE_CLASS": "registry-specific", "GIT_SERVER_STORAGE_CLASS": "",
	}}
	require.NoError(t, configurePackageDeployOptions(&opts, pkg))
	require.Equal(t, value.Values{"persistence": map[string]any{"storageClass": "registry-specific"}}, opts.ValuesOverridesMap["zarf-registry"]["docker-registry"])
	require.Equal(t, value.Values{"persistence": map[string]any{"storageClass": ""}}, opts.ValuesOverridesMap["git-server"]["gitea"])
}

func TestOldAlphaInitStorageClassOverrideScope(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name         string
		kind         api.PackageKind
		storageClass string
		component    string
		chart        string
	}{
		{"unset storage class", api.ZarfInitConfig, "", "zarf-registry", "docker-registry"},
		{"ordinary package", api.ZarfPackageConfig, "custom-storage", "zarf-registry", "docker-registry"},
		{"unrelated component", api.ZarfInitConfig, "custom-storage", "other", "docker-registry"},
		{"unrelated chart", api.ZarfInitConfig, "custom-storage", "zarf-registry", "other"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pkg := api.Package{
				APIVersion: v1alpha1.APIVersion, Kind: tt.kind,
				Components: []api.Component{{Name: tt.component, Charts: []api.Chart{{Name: tt.chart}}}},
			}
			opts := packager.DeployOptions{SetVariables: map[string]string{config.StorageClass: tt.storageClass}}
			require.NoError(t, configurePackageDeployOptions(&opts, pkg))
			require.Nil(t, opts.ValuesOverridesMap)
		})
	}
}

func TestConfigureInitDeployOptions(t *testing.T) {
	for _, version := range []string{v1alpha1.APIVersion, v1beta1.APIVersion} {
		t.Run(version, func(t *testing.T) {
			vars := map[string]string{
				config.RegistryURL:          "registry.example.com",
				config.RegistryPushPassword: "test-password",
				config.GitURL:               "https://git.example.com",
				config.InjectorImage:        "registry.example.com/injector:test",
				config.StorageClass:         "custom-storage",
			}
			opts := packager.DeployOptions{SetVariables: vars}
			pkg := api.Package{APIVersion: version, Kind: api.ZarfInitConfig}
			require.NoError(t, configurePackageDeployOptions(&opts, pkg))
			require.NoError(t, opts.Validate(pkg))
			if version == v1beta1.APIVersion {
				require.Empty(t, opts.SetVariables)
				require.Equal(t, value.Values{
					"registry":  map[string]any{"persistence": map[string]any{"storageClass": "custom-storage"}},
					"gitServer": map[string]any{"persistence": map[string]any{"storageClass": "custom-storage"}},
				}, opts.Values)
			} else {
				require.Equal(t, "custom-storage", opts.SetVariables["REGISTRY_STORAGE_CLASS"])
				require.Equal(t, "custom-storage", opts.SetVariables["GIT_SERVER_STORAGE_CLASS"])
			}
			require.Len(t, vars, 5, "caller variables must remain unchanged")
		})
	}
}
