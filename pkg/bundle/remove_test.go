// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"slices"
	"testing"

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
	RemovedPackages []*Package

	// NotDeployed lists package names that should return ErrPackageNotDeployed.
	NotDeployed []string

	// FailOnPackage causes RemovePackage to fail for a specific package name.
	FailOnPackage string

	// FailError is the error to return when FailOnPackage matches.
	FailError error
}

func (m *mockRemover) RemovePackage(_ context.Context, pkg *Package, _ RemovePackageOptions) error {
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

func defaultPkgOpts() RemovePackageOptions {
	return RemovePackageOptions{
		Config: &UDSBundleConfig{Global: &GlobalOptions{}, Options: &ConfigOptions{}},
	}
}

func TestRemovePackages_ReverseLevelOrder(t *testing.T) {
	mock := &mockRemover{}
	// 3 sequential levels: core -> nginx -> podinfo
	levels := makeLevels([]string{"core"}, []string{"nginx"}, []string{"podinfo"})

	removed, skipped, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, nil, levels, defaultPkgOpts())
	require.NoError(t, err)
	assert.Equal(t, 3, removed)
	assert.Equal(t, 0, skipped)
	// Reverse topological order: dependents (last level) removed first
	assert.Equal(t, []string{"podinfo", "nginx", "core"}, mock.removedNames())
}

func TestRemovePackages_DiamondReverseOrder(t *testing.T) {
	mock := &mockRemover{}
	// Diamond: level0=[a], level1=[b,c], level2=[d]
	levels := makeLevels([]string{"a"}, []string{"b", "c"}, []string{"d"})

	removed, _, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, nil, levels, defaultPkgOpts())
	require.NoError(t, err)
	assert.Equal(t, 4, removed)
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

	removed, skipped, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, nil, levels, defaultPkgOpts())
	require.NoError(t, err)
	assert.Equal(t, 2, removed)
	assert.Equal(t, 1, skipped)
	assert.Equal(t, []string{"podinfo", "core"}, mock.removedNames())
}

func TestRemovePackages_StopsOnError(t *testing.T) {
	mock := &mockRemover{
		FailOnPackage: "nginx",
		FailError:     fmt.Errorf("helm timeout"),
	}
	levels := makeLevels([]string{"core"}, []string{"nginx"}, []string{"podinfo"})

	removed, skipped, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, nil, levels, defaultPkgOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove package")
	assert.Contains(t, err.Error(), "nginx")
	// podinfo removed first (last level), then nginx failed
	assert.Equal(t, 1, removed)
	assert.Equal(t, 0, skipped)
}

func TestRemovePackages_NoneDeployed(t *testing.T) {
	mock := &mockRemover{NotDeployed: []string{"core", "nginx"}}
	levels := makeLevels([]string{"core"}, []string{"nginx"})

	removed, skipped, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, nil, levels, defaultPkgOpts())
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
	assert.Equal(t, 2, skipped)
	assert.Empty(t, mock.removedNames())
}

func TestRemovePackages_PropagatesNonSentinelError(t *testing.T) {
	mock := &mockRemover{
		FailOnPackage: "core",
		FailError:     fmt.Errorf("connection refused"),
	}
	levels := makeLevels([]string{"core"})

	_, _, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, nil, levels, defaultPkgOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove package")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestRemovePackages_EmptyLevels(t *testing.T) {
	mock := &mockRemover{}

	removed, skipped, err := withMock(mock).removePackages(t.Context(), iostreams.IOStreams{}, nil, nil, defaultPkgOpts())
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
	assert.Equal(t, 0, skipped)
}

func TestRemove_NilConfig(t *testing.T) {
	_, err := Remove(t.Context(), RemoveOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
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
