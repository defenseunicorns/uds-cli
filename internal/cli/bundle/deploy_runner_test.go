// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunDeployWith_PropagatesConfigAndClosesSource(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	bundlePath := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundleFileName)
	baseConfig := testDeployBaseConfig(7)
	closeCalls := 0
	prepared := &preparedDeploySource{source: &bundlepkg.DeploySource{BundlePath: bundlePath}, close: func() error {
		closeCalls++
		return nil
	}}
	deployCalls := 0

	result, err := runDeployWith(t.Context(), streams, baseConfig, bundlePath, []string{"init"}, true, false, deployRunnerDependencies{
		prepare: func(_ context.Context, _ iostreams.IOStreams, gotPath, tmpDir, architecture string) (*preparedDeploySource, error) {
			assert.Equal(t, bundlePath, gotPath)
			assert.Equal(t, baseConfig.Options.TmpDir, tmpDir)
			assert.Equal(t, baseConfig.Options.Architecture, architecture)
			return prepared, nil
		},
		deploy: func(_ context.Context, source *bundlepkg.DeploySource, opts bundlepkg.DeployOptions) (*bundlepkg.DeployResult, error) {
			deployCalls++
			assert.Same(t, prepared.source, source)
			assert.Equal(t, bundlePath, source.BundlePath)
			assert.Equal(t, []string{"init"}, opts.Packages)
			assert.True(t, opts.Force)
			assert.Equal(t, 7, opts.Config.Options.Concurrency)
			assert.Equal(t, "config", opts.Config.Variables["from_config"])
			return &bundlepkg.DeployResult{BundleName: "k3d-core-init", Packages: []bundlepkg.DeployPackageResult{{Name: "init"}}}, nil
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "k3d-core-init", result.BundleName)
	assert.Equal(t, 1, deployCalls)
	assert.Equal(t, 1, closeCalls)
}

func TestRunDeployWith_ForceOnlyBypassesDependencySafety(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	bundlePath := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundleFileName)

	for _, tt := range []struct {
		name            string
		packages        []string
		force           bool
		wantErr         string
		wantDeployCalls int
	}{
		{name: "dependency rejected", packages: []string{"init"}, wantErr: "unselected dependencies"},
		{name: "dependency forced", packages: []string{"init"}, force: true, wantDeployCalls: 1},
		{name: "unknown package remains rejected", packages: []string{"missing"}, force: true, wantErr: "unknown packages"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			closeCalls := 0
			deployCalls := 0
			_, err := runDeployWith(t.Context(), streams, testDeployBaseConfig(3), bundlePath, tt.packages, tt.force, false, deployRunnerDependencies{
				prepare: func(context.Context, iostreams.IOStreams, string, string, string) (*preparedDeploySource, error) {
					return &preparedDeploySource{source: &bundlepkg.DeploySource{BundlePath: bundlePath}, close: func() error { closeCalls++; return nil }}, nil
				},
				deploy: func(context.Context, *bundlepkg.DeploySource, bundlepkg.DeployOptions) (*bundlepkg.DeployResult, error) {
					deployCalls++
					return &bundlepkg.DeployResult{}, nil
				},
			})
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
			assert.Equal(t, tt.wantDeployCalls, deployCalls)
			assert.Equal(t, 1, closeCalls)
		})
	}
}

func TestRunDeployWith_ClosesSourceOnDeployError(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	bundlePath := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundleFileName)
	closeCalls := 0

	_, err := runDeployWith(t.Context(), streams, testDeployBaseConfig(2), bundlePath, nil, false, false, deployRunnerDependencies{
		prepare: func(context.Context, iostreams.IOStreams, string, string, string) (*preparedDeploySource, error) {
			return &preparedDeploySource{source: &bundlepkg.DeploySource{BundlePath: bundlePath}, close: func() error { closeCalls++; return nil }}, nil
		},
		deploy: func(context.Context, *bundlepkg.DeploySource, bundlepkg.DeployOptions) (*bundlepkg.DeployResult, error) {
			return nil, fmt.Errorf("cluster unavailable")
		},
	})

	require.ErrorContains(t, err, "deployment failed: cluster unavailable")
	assert.Equal(t, 1, closeCalls)
}

func testDeployBaseConfig(concurrency int) *bundlepkg.UDSBundleConfig {
	return &bundlepkg.UDSBundleConfig{
		Options: &bundlepkg.ConfigOptions{
			LogLevel:     "info",
			Architecture: "amd64",
			Concurrency:  concurrency,
			TmpDir:       "/tmp",
		},
		Variables: bundlepkg.Variables{"from_config": "config"},
	}
}
