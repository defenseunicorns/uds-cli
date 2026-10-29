// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package sources contains Zarf packager sources
package sources

import (
	"context"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/utils"
	"github.com/opencontainers/go-digest"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// addNamespaceOverrides applies chart namespace overrides to a package definition.
func addNamespaceOverrides(pkgLayout *layout.PackageLayout, nsOverrides NamespaceOverrideMap) {
	for componentName, chartOverrides := range nsOverrides {
		for chartName, namespace := range chartOverrides {
			pkgLayout.SetChartNamespace(componentName, chartName, namespace)
		}
	}
}

func loadPackageFromDir(ctx context.Context, dirPath string, opts layout.PackageLayoutOptions, manifestDigest digest.Digest) (*layout.PackageLayout, error) {
	pkgLayout, err := utils.LoadPackageFromDir(ctx, dirPath, opts)
	if err != nil {
		return nil, err
	}
	pkgLayout.SetRegistryDigest(manifestDigest.String())
	return pkgLayout, nil
}

type PackageSource interface {
	// LoadPackage loads a package from a source.
	LoadPackage(ctx context.Context, filter filters.ComponentFilterStrategy) (pkgLayout *layout.PackageLayout, warnings []string, err error)

	// LoadPackageMetadata loads a package's metadata from a source.
	LoadPackageMetadata(ctx context.Context, wantSBOM bool, skipValidation bool) (pkg api.Package, warnings []string, err error)
}

// handleFilter filters components and checks if a package is a partial package by checking its number of components
func handleFilter(pkg api.Package, filter filters.ComponentFilterStrategy) ([]api.Component, bool, error) {
	numComponents := len(pkg.Components)
	filteredDefinition, err := filters.Apply(pkg, filter)
	if err != nil {
		return nil, false, err
	}
	filteredComps := filteredDefinition.Components
	return filteredComps, numComponents > len(filteredComps), nil
}
