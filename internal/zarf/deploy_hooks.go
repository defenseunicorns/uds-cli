// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"

	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// withDefaults returns a PackageDeployHooks with nil funcs replaced by no-ops.
// Every deploy invokes both hooks through this normaliser, so callers always
// traverse both call sites regardless of whether custom hooks are installed.
func (h PackageDeployHooks) withDefaults() PackageDeployHooks {
	if h.PreDeploy == nil {
		h.PreDeploy = func(context.Context, *Package, *layout.PackageLayout, *packager.DeployOptions, *DeployPackageOptions) error {
			return nil
		}
	}
	if h.PostDeploy == nil {
		h.PostDeploy = func(context.Context, *Package) error {
			return nil
		}
	}
	return h
}

// withDefaults returns a BundleDeployHooks with nil funcs replaced by no-ops.
func (h BundleDeployHooks) withDefaults() BundleDeployHooks {
	if h.PreDeploy == nil {
		h.PreDeploy = func(context.Context, *UDSBundle, *DeployOptions) error {
			return nil
		}
	}
	if h.PostDeploy == nil {
		h.PostDeploy = func(context.Context, *UDSBundle) error {
			return nil
		}
	}
	return h
}
