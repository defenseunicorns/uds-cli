// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

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
