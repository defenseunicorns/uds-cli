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
	bundlePath := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundlepkg.BundleFileName)
	baseConfig := testDeployBaseConfig(7)
	prepared := &bundlepkg.DeploySource{BundlePath: bundlePath}
	closeCalls := 0
	deployCalls := 0

	result, err := runDeployWith(t.Context(), streams, baseConfig, bundlePath, []string{"init"}, true, bundlepkg.VerificationPolicy{}, false, deployRunnerDependencies{
		prepare: func(_ context.Context, opts bundlepkg.PrepareDeploySourceOptions) (*bundlepkg.DeploySource, error) {
			assert.Equal(t, bundlePath, opts.Path)
			assert.Equal(t, baseConfig.Options.TmpDir, opts.TmpDir)
			return prepared, nil
		},
		close: func(source *bundlepkg.DeploySource) error {
			closeCalls++
			assert.Same(t, prepared, source)
			return nil
		},
		deploy: func(_ context.Context, opts bundlepkg.DeployOptions) (*bundlepkg.DeployResult, error) {
			deployCalls++
			assert.Same(t, prepared, opts.Source)
			assert.Equal(t, bundlePath, opts.BundlePath)
			assert.Equal(t, []string{"init"}, opts.Packages)
			assert.True(t, opts.Force)
			assert.Equal(t, 7, opts.Config.Options.Concurrency)
			assert.Equal(t, "config", opts.Config.Variables["from_config"])
			return &bundlepkg.DeployResult{BundleName: "k3d-core-init", Packages: 1}, nil
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
	bundlePath := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundlepkg.BundleFileName)

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
			_, err := runDeployWith(t.Context(), streams, testDeployBaseConfig(3), bundlePath, tt.packages, tt.force, bundlepkg.VerificationPolicy{}, false, deployRunnerDependencies{
				prepare: func(context.Context, bundlepkg.PrepareDeploySourceOptions) (*bundlepkg.DeploySource, error) {
					return &bundlepkg.DeploySource{BundlePath: bundlePath}, nil
				},
				close: func(*bundlepkg.DeploySource) error {
					closeCalls++
					return nil
				},
				deploy: func(context.Context, bundlepkg.DeployOptions) (*bundlepkg.DeployResult, error) {
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
	bundlePath := filepath.Join("..", "..", "..", "tests", "test_data", "bundles", "deploy", "init", bundlepkg.BundleFileName)
	closeCalls := 0

	_, err := runDeployWith(t.Context(), streams, testDeployBaseConfig(2), bundlePath, nil, false, bundlepkg.VerificationPolicy{}, false, deployRunnerDependencies{
		prepare: func(context.Context, bundlepkg.PrepareDeploySourceOptions) (*bundlepkg.DeploySource, error) {
			return &bundlepkg.DeploySource{BundlePath: bundlePath}, nil
		},
		close: func(*bundlepkg.DeploySource) error {
			closeCalls++
			return nil
		},
		deploy: func(context.Context, bundlepkg.DeployOptions) (*bundlepkg.DeployResult, error) {
			return nil, fmt.Errorf("cluster unavailable")
		},
	})

	require.ErrorContains(t, err, "deployment failed: cluster unavailable")
	assert.Equal(t, 1, closeCalls)
}

func testDeployBaseConfig(concurrency int) *bundlepkg.UDSBundleConfig {
	return &bundlepkg.UDSBundleConfig{
		Global: &bundlepkg.GlobalOptions{LogLevel: "info"},
		Options: &bundlepkg.ConfigOptions{
			Architecture: "amd64",
			Concurrency:  concurrency,
			TmpDir:       "/tmp",
		},
		Variables: bundlepkg.Variables{"from_config": "config"},
	}
}
