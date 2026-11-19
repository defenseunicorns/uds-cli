// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/state"
)

func filterResumePackages(packages []types.Package, deployed []state.DeployedPackage, loadMetadata func(types.Package) (api.Package, error)) ([]types.Package, error) {
	byIdentity := make(map[string][]state.DeployedPackage, len(deployed))
	for _, record := range deployed {
		id := deployedPackageLifecycleID(record)
		byIdentity[id] = append(byIdentity[id], record)
	}

	remaining := make([]types.Package, 0, len(packages))
	for _, pkg := range packages {
		digest, err := packageManifestDigest(pkg)
		if err != nil {
			return nil, err
		}
		definition, err := loadMetadata(pkg)
		if err != nil {
			return nil, fmt.Errorf("read package %q metadata for resume: %w", pkg.Name, err)
		}
		definition, err = filters.Apply(definition, filters.ForDeploy(strings.Join(pkg.OptionalComponents, ",")))
		if err != nil {
			return nil, fmt.Errorf("filter package %q components for resume: %w", pkg.Name, err)
		}
		records := byIdentity[bundlePackageLifecycleID(pkg)]
		if len(records) == 1 && records[0].Digest == digest.String() && resumeDeploymentMatches(definition.Components, &records[0]) {
			continue
		}
		remaining = append(remaining, pkg)
	}
	return remaining, nil
}

func resumeDeploymentMatches(intended []api.Component, record *state.DeployedPackage) bool {
	if event, ok := record.LatestEvent(); ok && (event.Type != state.EventTypeDeploy || event.Outcome != state.EventOutcomeSucceeded) {
		return false
	}
	if len(intended) != len(record.DeployedComponents) {
		return false
	}
	names := make(map[string]struct{}, len(intended))
	for _, component := range intended {
		if _, duplicate := names[component.Name]; duplicate || component.Name == "" {
			return false
		}
		names[component.Name] = struct{}{}
	}
	for _, component := range record.DeployedComponents {
		if _, found := names[component.Name]; !found {
			return false
		}
		if event := component.LastEvent; event.Type != "" {
			if event.Type != state.EventTypeDeploy || event.Outcome != state.EventOutcomeSucceeded {
				return false
			}
		} else if component.Status != state.ComponentStatusSucceeded { //nolint:staticcheck // Read deployment records saved before component events existed.
			return false
		}
		delete(names, component.Name)
	}
	return len(names) == 0
}
