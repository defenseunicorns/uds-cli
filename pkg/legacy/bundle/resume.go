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
		definition, err = filters.Apply(definition, filters.ForDeploy(strings.Join(pkg.OptionalComponents, ","), false))
		if err != nil {
			return nil, fmt.Errorf("filter package %q components for resume: %w", pkg.Name, err)
		}
		records := byIdentity[bundlePackageLifecycleID(pkg)]
		if len(records) == 1 && records[0].Digest == digest.String() && resumeComponentsMatch(definition.Components, records[0].DeployedComponents) {
			continue
		}
		remaining = append(remaining, pkg)
	}
	return remaining, nil
}

func resumeComponentsMatch(intended []api.Component, deployed []state.DeployedComponent) bool {
	if len(intended) != len(deployed) {
		return false
	}
	names := make(map[string]struct{}, len(intended))
	for _, component := range intended {
		if _, duplicate := names[component.Name]; duplicate || component.Name == "" {
			return false
		}
		names[component.Name] = struct{}{}
	}
	for _, component := range deployed {
		if _, found := names[component.Name]; !found || component.Status != state.ComponentStatusSucceeded {
			return false
		}
		delete(names, component.Name)
	}
	return len(names) == 0
}
