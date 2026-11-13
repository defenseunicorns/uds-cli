// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"slices"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/pkg/state"
)

func TestDeployedPackageSuccessUsesLatestDeployEvent(t *testing.T) {
	t.Parallel()
	for _, outcome := range []state.EventOutcome{state.EventOutcomeInProgress, state.EventOutcomeFailed, state.EventOutcomeSucceeded} {
		t.Run(string(outcome), func(t *testing.T) {
			pkg := state.DeployedPackage{DeployedComponents: []state.DeployedComponent{{Name: "app", Status: state.ComponentStatusSucceeded}}}
			require.NoError(t, pkg.SetPackageDefinition(api.Package{Components: []api.Component{{Name: "app"}}}))
			pkg.AppendEvent(state.PackageEvent{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeSucceeded})
			pkg.AppendEvent(state.PackageEvent{Type: state.EventTypeDeploy, Outcome: outcome})
			require.Equal(t, outcome == state.EventOutcomeSucceeded, deployedPackageIsSuccessful(pkg))
		})
	}
}

func TestDeployedPackageSuccessUsesComponentEvents(t *testing.T) {
	t.Parallel()
	for _, outcome := range []state.EventOutcome{state.EventOutcomeInProgress, state.EventOutcomeFailed, state.EventOutcomeSucceeded} {
		t.Run(string(outcome), func(t *testing.T) {
			pkg := state.DeployedPackage{DeployedComponents: []state.DeployedComponent{{
				Name:      "app",
				LastEvent: state.ComponentEvent{Type: state.EventTypeDeploy, Outcome: outcome},
			}}}
			require.NoError(t, pkg.SetPackageDefinition(api.Package{Components: []api.Component{{Name: "app"}}}))
			require.Equal(t, outcome == state.EventOutcomeSucceeded, deployedPackageIsSuccessful(pkg))
		})
	}
}

func TestDeployedPackageRemovalIsNotSuccessfulDeployment(t *testing.T) {
	t.Parallel()
	for _, outcome := range []state.EventOutcome{state.EventOutcomeInProgress, state.EventOutcomeFailed, state.EventOutcomeSucceeded} {
		t.Run(string(outcome), func(t *testing.T) {
			pkg := state.DeployedPackage{DeployedComponents: []state.DeployedComponent{{
				Name:      "app",
				LastEvent: state.ComponentEvent{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeSucceeded},
			}}}
			require.NoError(t, pkg.SetPackageDefinition(api.Package{Components: []api.Component{{Name: "app"}}}))
			pkg.AppendEvent(state.PackageEvent{Type: state.EventTypeRemove, Outcome: outcome})
			require.False(t, deployedPackageIsSuccessful(pkg))
		})
	}
}

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
