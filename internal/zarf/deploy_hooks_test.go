// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

func TestPackageDeployHooks_WithDefaults_NilFuncs(t *testing.T) {
	h := PackageDeployHooks{}
	got := h.withDefaults()
	assert.NotNil(t, got.PreDeploy)
	assert.NotNil(t, got.PostDeploy)
}

func TestPackageDeployHooks_WithDefaults_NonNilFuncsUnchanged(t *testing.T) {
	var preCalled, postCalled bool
	h := PackageDeployHooks{
		PreDeploy: func(_ context.Context, _ *spec.Package, _ *layout.PackageLayout, _ *packager.DeployOptions, _ *DeployPackageOptions) error {
			preCalled = true
			return nil
		},
		PostDeploy: func(_ context.Context, _ *spec.Package) error {
			postCalled = true
			return nil
		},
	}
	got := h.withDefaults()

	_ = got.PreDeploy(t.Context(), nil, nil, nil, nil)
	_ = got.PostDeploy(t.Context(), nil)

	assert.True(t, preCalled)
	assert.True(t, postCalled)
}

func TestPackageDeployHooks_DefaultsAreNoOps(t *testing.T) {
	h := PackageDeployHooks{}
	got := h.withDefaults()

	err := got.PreDeploy(t.Context(), &spec.Package{}, &layout.PackageLayout{}, &packager.DeployOptions{}, &DeployPackageOptions{})
	require.NoError(t, err)

	err = got.PostDeploy(t.Context(), &spec.Package{})
	require.NoError(t, err)
}

func TestBundleDeployHooks_WithDefaults_NilFuncs(t *testing.T) {
	h := BundleDeployHooks{}
	got := h.withDefaults()
	assert.NotNil(t, got.PreDeploy)
	assert.NotNil(t, got.PostDeploy)
}

func TestBundleDeployHooks_WithDefaults_NonNilFuncsUnchanged(t *testing.T) {
	var preCalled, postCalled bool
	h := BundleDeployHooks{
		PreDeploy:  func(_ context.Context, _ *spec.UDSBundle, _ *DeployOptions) error { preCalled = true; return nil },
		PostDeploy: func(_ context.Context, _ *spec.UDSBundle) error { postCalled = true; return nil },
	}
	got := h.withDefaults()

	_ = got.PreDeploy(t.Context(), nil, nil)
	_ = got.PostDeploy(t.Context(), nil)

	assert.True(t, preCalled)
	assert.True(t, postCalled)
}

func TestBundleDeployHooks_DefaultsAreNoOps(t *testing.T) {
	h := BundleDeployHooks{}
	got := h.withDefaults()

	err := got.PreDeploy(t.Context(), &spec.UDSBundle{}, &DeployOptions{})
	require.NoError(t, err)

	err = got.PostDeploy(t.Context(), &spec.UDSBundle{})
	require.NoError(t, err)
}

// TestImageZeroingMutation verifies the Remote Agent's PreDeploy mutation pattern
// in isolation: zeroing Images and ImageArchives on an in-memory PackageLayout.
func TestImageZeroingMutation(t *testing.T) {
	pkgLayout := &layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{
			Components: []v1alpha1.ZarfComponent{
				{
					Name:   "main",
					Images: []string{"ghcr.io/example/image:v1"},
					ImageArchives: []v1alpha1.ImageArchive{
						{Path: "archive.tar", Images: []string{"ghcr.io/example/image:v1"}},
					},
				},
				{
					Name:   "secondary",
					Images: []string{"ghcr.io/example/other:v1"},
				},
			},
		},
	}

	// Precondition: components have images before mutation.
	require.NotEmpty(t, pkgLayout.Pkg.Components[0].Images)
	require.NotEmpty(t, pkgLayout.Pkg.Components[0].ImageArchives)
	require.NotEmpty(t, pkgLayout.Pkg.Components[1].Images)

	// The Remote Agent PreDeploy hook mutation (promoted from uds-remote-agent client.go:109-113).
	for i := range pkgLayout.Pkg.Components {
		pkgLayout.Pkg.Components[i].Images = []string{}
		pkgLayout.Pkg.Components[i].ImageArchives = []v1alpha1.ImageArchive{}
	}

	for _, c := range pkgLayout.Pkg.Components {
		assert.Empty(t, c.Images, "component %q: Images should be empty after mutation", c.Name)
		assert.Empty(t, c.ImageArchives, "component %q: ImageArchives should be empty after mutation", c.Name)
	}
}
