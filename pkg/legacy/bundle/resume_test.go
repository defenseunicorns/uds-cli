// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"errors"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/pkg/state"
)

func TestFilterResumePackagesInterruptedUpgrade(t *testing.T) {
	oldDigest := "sha256:" + strings.Repeat("a", 64)
	targetDigest := "sha256:" + strings.Repeat("b", 64)
	packages := []types.Package{
		{Name: "first", Ref: "2.0.0@" + targetDigest},
		{Name: "second", Ref: "2.0.0@" + targetDigest},
		{Name: "third", Ref: "2.0.0@" + targetDigest},
	}
	definition := api.Package{Components: []api.Component{{Name: "main"}}}
	deployed := make([]state.DeployedPackage, 0, len(packages))
	for i, pkg := range packages {
		digest := oldDigest
		if i == 0 {
			digest = targetDigest
		}
		record := state.DeployedPackage{
			Name: pkg.Name, Digest: digest,
			DeployedComponents: []state.DeployedComponent{{Name: "main", Status: state.ComponentStatusSucceeded}},
		}
		require.NoError(t, record.SetPackageDefinition(definition))
		deployed = append(deployed, record)
	}
	loadMetadata := func(types.Package) (api.Package, error) { return definition, nil }

	remaining, err := filterResumePackages(packages, deployed, loadMetadata)
	require.NoError(t, err)
	require.Equal(t, packages[1:], remaining, "resume must retain packages still running the older deployment, in bundle order")

	for i := range deployed {
		deployed[i].Digest = targetDigest
	}
	remaining, err = filterResumePackages(packages, deployed, loadMetadata)
	require.NoError(t, err)
	require.Empty(t, remaining, "a completed upgrade must not redeploy packages")
}

func TestFilterResumePackagesMatching(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	pkg := types.Package{Name: "example", Namespace: "team-a", Ref: "1.0.0@" + digest}
	definition := api.Package{Components: []api.Component{{Name: "main"}, {Name: "extra", Optional: true}}}
	matching := state.DeployedPackage{
		Name: pkg.Name, NamespaceOverride: pkg.Namespace, Digest: digest,
		DeployedComponents: []state.DeployedComponent{{Name: "main", Status: state.ComponentStatusSucceeded}},
	}
	tests := []struct {
		name   string
		change func(*types.Package, *state.DeployedPackage)
		skip   bool
	}{
		{name: "exact successful match", skip: true},
		{name: "different digest", change: func(_ *types.Package, record *state.DeployedPackage) {
			record.Digest = "sha256:" + strings.Repeat("b", 64)
		}},
		{name: "missing digest", change: func(_ *types.Package, record *state.DeployedPackage) { record.Digest = "" }},
		{name: "different namespace", change: func(_ *types.Package, record *state.DeployedPackage) { record.NamespaceOverride = "team-b" }},
		{name: "different name", change: func(_ *types.Package, record *state.DeployedPackage) { record.Name = "other" }},
		{name: "failed component", change: func(_ *types.Package, record *state.DeployedPackage) {
			record.DeployedComponents = []state.DeployedComponent{{Name: "main", Status: state.ComponentStatusFailed}}
		}},
		{name: "missing component", change: func(_ *types.Package, record *state.DeployedPackage) { record.DeployedComponents = nil }},
		{name: "different component", change: func(_ *types.Package, record *state.DeployedPackage) {
			record.DeployedComponents = []state.DeployedComponent{{Name: "other", Status: state.ComponentStatusSucceeded}}
		}},
		{name: "new optional selection", change: func(pkg *types.Package, _ *state.DeployedPackage) { pkg.OptionalComponents = []string{"extra"} }},
		{name: "removed optional selection", change: func(_ *types.Package, record *state.DeployedPackage) {
			record.DeployedComponents = append(record.DeployedComponents, state.DeployedComponent{Name: "extra", Status: state.ComponentStatusSucceeded})
		}},
		{name: "matching optional selection", skip: true, change: func(pkg *types.Package, record *state.DeployedPackage) {
			pkg.OptionalComponents = []string{"extra"}
			record.DeployedComponents = append(record.DeployedComponents, state.DeployedComponent{Name: "extra", Status: state.ComponentStatusSucceeded})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, record := pkg, matching
			if tt.change != nil {
				tt.change(&pkg, &record)
			}
			remaining, err := filterResumePackages([]types.Package{pkg}, []state.DeployedPackage{record}, func(types.Package) (api.Package, error) {
				return definition, nil
			})
			require.NoError(t, err)
			if tt.skip {
				require.Empty(t, remaining)
			} else {
				require.Equal(t, []types.Package{pkg}, remaining)
			}
		})
	}
	for _, records := range [][]state.DeployedPackage{nil, {matching, matching}} {
		remaining, err := filterResumePackages([]types.Package{pkg}, records, func(types.Package) (api.Package, error) { return definition, nil })
		require.NoError(t, err)
		require.Equal(t, []types.Package{pkg}, remaining, "missing or ambiguous state must not skip a package")
	}
}

func TestFilterResumePackagesMetadataError(t *testing.T) {
	packages := []types.Package{{Name: "example", Ref: "1.0.0@sha256:" + strings.Repeat("a", 64)}}
	readErr := errors.New("metadata unavailable")
	remaining, err := filterResumePackages(packages, nil, func(types.Package) (api.Package, error) { return api.Package{}, readErr })
	require.ErrorIs(t, err, readErr)
	require.Nil(t, remaining)
}
