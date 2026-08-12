// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZarfRemoverRemoveBundleValidatesRemovalSafety(t *testing.T) {
	b := &spec.UDSBundle{
		UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: spec.Metadata{Name: "example"},
		Packages: []spec.Package{
			{Name: "core", Source: "oci://example/core:1"},
			{Name: "app", Source: "oci://example/app:1", DependsOn: []spec.PackageRef{{Name: "core"}}},
		},
	}

	result, err := NewZarfRemover(iostreams.IOStreams{}).RemoveBundle(
		t.Context(), b, []string{"core"}, RemovePackageOptions{Config: newTestConfig()},
	)
	require.Error(t, err)
	assert.Nil(t, result)

	var dependencyErr *DependencyViolationError
	require.ErrorAs(t, err, &dependencyErr)
	assert.Equal(t, map[string][]string{"core": {"app"}}, dependencyErr.Violations)
}

func TestPublicAdaptersValidateOptionsBeforeOperationLogic(t *testing.T) {
	b := &spec.UDSBundle{
		Packages: []spec.Package{
			{Name: "core", Source: "oci://example/core:1"},
			{Name: "app", Source: "oci://example/app:1", DependsOn: []spec.PackageRef{{Name: "core"}}},
		},
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "pull bundle", run: func() error {
			_, err := NewDefaultPuller().PullBundle(t.Context(), "invalid", "", PullOptions{})
			return err
		}},
		{name: "pull package", run: func() error {
			_, err := NewDefaultPuller().PullPackage(t.Context(), "invalid", "", PullOptions{})
			return err
		}},
		{name: "push bundle", run: func() error {
			_, err := NewDefaultPusher().PushBundle(t.Context(), "", "invalid", PushOptions{})
			return err
		}},
		{name: "push package", run: func() error {
			_, err := NewDefaultPusher().PushPackage(t.Context(), "", "invalid", PushOptions{})
			return err
		}},
		{name: "deploy package", run: func() error {
			return NewZarfDeployer(iostreams.IOStreams{}, nil).DeployPackage(t.Context(), &spec.Package{}, DeployPackageOptions{})
		}},
		{name: "deploy bundle", run: func() error {
			_, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(
				t.Context(), b, DeployOptions{Packages: []string{"app"}},
			)
			return err
		}},
		{name: "remove package", run: func() error {
			return NewZarfRemover(iostreams.IOStreams{}).RemovePackage(t.Context(), &spec.Package{}, RemovePackageOptions{})
		}},
		{name: "remove bundle", run: func() error {
			_, err := NewZarfRemover(iostreams.IOStreams{}).RemoveBundle(
				t.Context(), b, []string{"core"}, RemovePackageOptions{},
			)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, tt.run(), "config is required")
		})
	}
}
