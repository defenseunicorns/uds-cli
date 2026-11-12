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
