// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
	"maps"

	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/api/v1beta1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/value"
)

func configurePackageDeployOptions(opts *packager.DeployOptions, pkg api.Package) error {
	isBeta := pkg.GetAPIVersion() == v1beta1.APIVersion
	if pkg.IsInitConfig() {
		opts.SetVariables = maps.Clone(opts.SetVariables)
		if storageClass := newStorageClass(opts.SetVariables, pkg.Kind); storageClass != "" {
			if isBeta {
				opts.Values = opts.Values.DeepCopy()
				for _, path := range []value.Path{".registry.persistence.storageClass", ".gitServer.persistence.storageClass"} {
					if err := opts.Values.Set(path, storageClass); err != nil {
						return fmt.Errorf("set init storage class: %w", err)
					}
				}
			} else {
				// Zarf no longer stores a global storage class in cluster state.
				// Configure each v1alpha1 init component through its declared variable.
				for _, key := range []string{"REGISTRY_STORAGE_CLASS", "GIT_SERVER_STORAGE_CLASS"} {
					if _, exists := opts.SetVariables[key]; !exists {
						opts.SetVariables[key] = storageClass
					}
				}
				configureLegacyInitChartStorageClasses(opts, pkg)
			}
		}
	}
	if isBeta {
		// Legacy variables have already supplied init SDK settings and rendered
		// bundle chart overrides. v1beta1 accepts package values instead of variables.
		opts.SetVariables = nil
	}
	return opts.Validate(pkg)
}

func configureLegacyInitChartStorageClasses(opts *packager.DeployOptions, pkg api.Package) {
	declared := make(map[string]bool)
	for _, variable := range pkg.Variables { //nolint:staticcheck // Detect alpha init packages that declare the replacement storage-class variables.
		declared[variable.Name] = true
	}
	targets := map[string]struct{ chart, variable string }{
		"zarf-seed-registry": {"docker-registry", "REGISTRY_STORAGE_CLASS"},
		"zarf-registry":      {"docker-registry", "REGISTRY_STORAGE_CLASS"},
		"git-server":         {"gitea", "GIT_SERVER_STORAGE_CLASS"},
	}
	for _, component := range pkg.Components {
		target, exists := targets[component.Name]
		if !exists || declared[target.variable] {
			continue
		}
		for _, chart := range component.Charts {
			if chart.Name != target.chart {
				continue
			}
			// Older init charts use the removed ###ZARF_STORAGE_CLASS###
			// placeholder. Supply a default while preserving explicit chart overrides.
			values := value.Values{"persistence": map[string]any{"storageClass": opts.SetVariables[target.variable]}}
			values.DeepMerge(opts.ValuesOverridesMap[component.Name][chart.Name])
			overrides := maps.Clone(opts.ValuesOverridesMap)
			if overrides == nil {
				overrides = make(packager.ValuesOverrides)
			}
			charts := maps.Clone(overrides[component.Name])
			if charts == nil {
				charts = make(map[string]value.Values)
			}
			charts[chart.Name] = values
			overrides[component.Name] = charts
			opts.ValuesOverridesMap = overrides
		}
	}
}
