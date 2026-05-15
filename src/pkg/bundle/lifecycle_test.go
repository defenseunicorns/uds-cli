// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/state"
	"golang.org/x/exp/slices"
)

func TestBundlePackageLifecycleID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "nginx", bundlePackageLifecycleID(types.Package{Name: "nginx"}))
	require.Equal(t, "nginx/package-override-ns", bundlePackageLifecycleID(types.Package{Name: "nginx", Namespace: "package-override-ns"}))
}

func TestDeployedPackageLifecycleID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "nginx", deployedPackageLifecycleID(state.DeployedPackage{Name: "nginx"}))
	require.Equal(t, "nginx/package-override-ns", deployedPackageLifecycleID(state.DeployedPackage{Name: "nginx", NamespaceOverride: "package-override-ns"}))
}

func TestLifecycleIDsTreatNamespaceOverridesSeparately(t *testing.T) {
	t.Parallel()

	deployedPackageIDs := []string{
		deployedPackageLifecycleID(state.DeployedPackage{Name: "nginx", NamespaceOverride: "package-a"}),
	}

	require.True(t, slices.Contains(deployedPackageIDs, bundlePackageLifecycleID(types.Package{Name: "nginx", Namespace: "package-a"})))
	require.False(t, slices.Contains(deployedPackageIDs, bundlePackageLifecycleID(types.Package{Name: "nginx", Namespace: "package-b"})))
	require.False(t, slices.Contains(deployedPackageIDs, bundlePackageLifecycleID(types.Package{Name: "nginx"})))
}
