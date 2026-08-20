// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

// Package bundle_test contains library-facing examples for bundle deployment.
package bundle_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	bundle "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLibraryDeploy_Showcase(t *testing.T) {
	var logs bytes.Buffer
	streams := iostreams.New(nil, nil, &logs)
	hookErr := errors.New("stop before cluster deployment")
	var startedBundle string
	var captured *bundle.ZarfPackageLayout

	result, err := bundle.Deploy(t.Context(), deploySource(singlePkgBundle()), bundle.DeployOptions{
		Config: func() *bundle.UDSBundleConfig {
			config := newTestConfig()
			config.Options.LogLevel = "info"
			return config
		}(),
		Streams: streams,
		BundleDeployHooks: bundle.BundleDeployHooks{
			PreDeploy: func(_ context.Context, b *spec.UDSBundle, _ *bundle.DeployOptions) error {
				startedBundle = b.Metadata.Name
				return nil
			},
		},
		PackageDeployHooks: bundle.PackageDeployHooks{
			PreDeploy: func(_ context.Context, _ *spec.Package, pkgLayout *bundle.ZarfPackageLayout, _ *bundle.DeployPackageOptions) error {
				captured = pkgLayout
				pkgLayout.Pkg.Components[0].Images = nil
				pkgLayout.Pkg.Components[0].ImageArchives = nil
				return hookErr
			},
		},
	})

	require.ErrorIs(t, err, hookErr)
	assert.Nil(t, result)
	assert.Equal(t, "test-bundle", startedBundle)
	require.NotNil(t, captured)
	assert.Empty(t, captured.Pkg.Components[0].Images)
	assert.Empty(t, captured.Pkg.Components[0].ImageArchives)
	assert.Contains(t, logs.String(), "deploying bundle")
}
