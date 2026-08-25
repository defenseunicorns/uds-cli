// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"fmt"
	"slices"
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRemover is a per-package test double injected into ZarfRemover via the
// pkgRemover field to exercise removePackages in isolation. Mirrors MockDeployer
// (deploy_test.go): records calls in order and supports failure injection.
// Packages listed in NotDeployed return ErrPackageNotDeployed to exercise the
// orchestrator's skip path.
type mockRemover struct {
	// RemovedPackages tracks the packages that were removed in order.
	RemovedPackages []*spec.Package

	// NotDeployed lists package names that should return ErrPackageNotDeployed.
	NotDeployed []string

	// FailOnPackage causes RemovePackage to fail for a specific package name.
	FailOnPackage string

	// FailError is the error to return when FailOnPackage matches.
	FailError error
}

// RemovePackage records a package removal or returns the configured test error.
func (m *mockRemover) RemovePackage(_ context.Context, pkg *spec.Package, _ RemovePackageOptions) error {
	if slices.Contains(m.NotDeployed, pkg.Name) {
		return ErrPackageNotDeployed
	}
	if m.FailOnPackage == pkg.Name && m.FailError != nil {
		return m.FailError
	}
	m.RemovedPackages = append(m.RemovedPackages, pkg)
	return nil
}

// removedNames returns the names of removed packages in call order.
func (m *mockRemover) removedNames() []string {
	names := make([]string, 0, len(m.RemovedPackages))
	for _, p := range m.RemovedPackages {
		names = append(names, p.Name)
	}
	return names
}

// withMock returns a ZarfRemover whose per-package primitive is the given mock,
// so removePackages can be exercised without a live cluster.
func withMock(m *mockRemover) *ZarfRemover {
	return &ZarfRemover{pkgRemover: m}
}

// defaultPkgOpts returns valid baseline package removal options.
func defaultPkgOpts() RemovePackageOptions {
	return RemovePackageOptions{
		Config: &UDSBundleConfig{Options: &bundleinternal.ConfigOptions{}},
	}
}

// makeLevels constructs dependency levels from package names.
func makeLevels(names ...[]string) [][]*spec.Package {
	levels := make([][]*spec.Package, len(names))
	for i, levelNames := range names {
		levels[i] = make([]*spec.Package, len(levelNames))
		for j, name := range levelNames {
			levels[i][j] = &spec.Package{Name: name}
		}
	}
	return levels
}

func TestRemovePackages_ReverseLevelOrder(t *testing.T) {
	mock := &mockRemover{}
	// 3 sequential levels: core -> nginx -> podinfo
	levels := makeLevels([]string{"core"}, []string{"nginx"}, []string{"podinfo"})

	results, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, levels, defaultPkgOpts())
	require.NoError(t, err)
	assert.Len(t, results, 3)
	// Reverse topological order: dependents (last level) removed first
	assert.Equal(t, []string{"podinfo", "nginx", "core"}, mock.removedNames())
}

func TestRemovePackages_DiamondReverseOrder(t *testing.T) {
	mock := &mockRemover{}
	// Diamond: level0=[a], level1=[b,c], level2=[d]
	levels := makeLevels([]string{"a"}, []string{"b", "c"}, []string{"d"})

	results, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, levels, defaultPkgOpts())
	require.NoError(t, err)
	assert.Len(t, results, 4)
	// d removed first (top of DAG), then b/c (siblings, any order), then a (root)
	names := mock.removedNames()
	require.Len(t, names, 4)
	assert.Equal(t, "d", names[0])
	assert.ElementsMatch(t, []string{"b", "c"}, names[1:3])
	assert.Equal(t, "a", names[3])
}

func TestRemovePackages_SkipsUndeployed(t *testing.T) {
	mock := &mockRemover{NotDeployed: []string{"nginx"}}
	levels := makeLevels([]string{"core"}, []string{"nginx"}, []string{"podinfo"})

	results, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, levels, defaultPkgOpts())
	require.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, RemovePackageStatusSkipped, results[1].Status)
	assert.Equal(t, []string{"podinfo", "core"}, mock.removedNames())
}

func TestRemovePackages_StopsOnError(t *testing.T) {
	mock := &mockRemover{
		FailOnPackage: "nginx",
		FailError:     fmt.Errorf("helm timeout"),
	}
	levels := makeLevels([]string{"core"}, []string{"nginx"}, []string{"podinfo"})

	results, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, levels, defaultPkgOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove package")
	assert.Contains(t, err.Error(), "nginx")
	// podinfo removed first (last level), then nginx failed
	assert.Len(t, results, 1)
}

func TestRemovePackages_NoneDeployed(t *testing.T) {
	mock := &mockRemover{NotDeployed: []string{"core", "nginx"}}
	levels := makeLevels([]string{"core"}, []string{"nginx"})

	results, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, levels, defaultPkgOpts())
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, RemovePackageStatusSkipped, results[0].Status)
	assert.Equal(t, RemovePackageStatusSkipped, results[1].Status)
	assert.Empty(t, mock.removedNames())
}

func TestRemovePackages_PropagatesNonSentinelError(t *testing.T) {
	mock := &mockRemover{
		FailOnPackage: "core",
		FailError:     fmt.Errorf("connection refused"),
	}
	levels := makeLevels([]string{"core"})

	_, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, levels, defaultPkgOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove package")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestRemovePackages_EmptyLevels(t *testing.T) {
	mock := &mockRemover{}

	results, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, nil, defaultPkgOpts())
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestZarfRemover_RemoveBundle_NilConfig(t *testing.T) {
	_, err := NewZarfRemover(iostreams.IOStreams{}).RemoveBundle(t.Context(), &spec.UDSBundle{}, nil, RemovePackageOptions{})
	require.ErrorContains(t, err, "config is required")
}

func TestZarfRemover_RemoveBundleFailureOmitsInvalidSourceRange(t *testing.T) {
	failErr := fmt.Errorf("helm timeout")
	mock := &mockRemover{FailOnPackage: "core", FailError: failErr}
	r := &ZarfRemover{streams: iostreams.IOStreams{}, pkgRemover: mock}
	b := &spec.UDSBundle{
		UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: spec.Metadata{Name: "example"},
		Packages: []spec.Package{{Name: "core", Source: "oci://example/core:1"}},
	}
	opts := defaultPkgOpts()
	opts.Config.Options.Concurrency = 1

	_, err := r.RemoveBundle(t.Context(), b, nil, opts)
	require.ErrorIs(t, err, failErr)
	assert.NotContains(t, err.Error(), ":0,0-0")
}

// TestDeployedKey locks in that the deployed-package cache key combines the
// Zarf metadata.name with the namespace override. Zarf state secrets are named
// zarf-package-<name> or zarf-package-<name>-override-<ns>, so the same Zarf
// package deployed twice into different namespaces produces distinct entries.
// Keying by name alone would collapse those.
func TestDeployedKey(t *testing.T) {
	tests := []struct {
		name     string
		left     [2]string // (zarfName, namespace)
		right    [2]string
		wantSame bool
	}{
		{
			name:     "same name and namespace collide (correct)",
			left:     [2]string{"core-base", "uds"},
			right:    [2]string{"core-base", "uds"},
			wantSame: true,
		},
		{
			name:     "same name with empty namespaces collide",
			left:     [2]string{"core-base", ""},
			right:    [2]string{"core-base", ""},
			wantSame: true,
		},
		{
			name:     "same name, different namespaces are distinct",
			left:     [2]string{"core-base", "uds"},
			right:    [2]string{"core-base", "monitoring"},
			wantSame: false,
		},
		{
			name:     "same name, empty vs set namespace are distinct",
			left:     [2]string{"core-base", ""},
			right:    [2]string{"core-base", "uds"},
			wantSame: false,
		},
		{
			name:     "different names, same namespace are distinct",
			left:     [2]string{"core-base", "uds"},
			right:    [2]string{"core-logging", "uds"},
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := deployedKey(tt.left[0], tt.left[1])
			r := deployedKey(tt.right[0], tt.right[1])
			if tt.wantSame {
				assert.Equal(t, l, r)
			} else {
				assert.NotEqual(t, l, r)
			}
		})
	}
}
