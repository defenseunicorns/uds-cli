// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package sources contains Zarf packager sources
package sources

import (
	"context"
	"fmt"

	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/layout"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
)

// ZarfState is a package source for Zarf packages that have already been deployed
type ZarfState struct {
	pkgName string
	state   *zarfTypes.DeployedPackage
}

func (z *ZarfState) LoadPackageMetadata(_ context.Context, _ *layout.PackagePaths, _ bool, _ bool) (pkg v1alpha1.ZarfPackage, warnings []string, err error) {
	if z.state != nil {
		return v1alpha1.ZarfPackage{}, nil, fmt.Errorf("missing metadata from deployed pkg: %s", z.pkgName)
	}
	return z.state.Data, nil, nil
}

// LoadPackage doesn't need to be implemented because this source is only used for package removal
func (z *ZarfState) LoadPackage(_ context.Context, _ *layout.PackagePaths, _ filters.ComponentFilterStrategy, _ bool) (v1alpha1.ZarfPackage, []string, error) {
	return v1alpha1.ZarfPackage{}, nil, fmt.Errorf("not implemented in %T", z)
}

// Collect doesn't need to be implemented because this source is only used for package removal
func (z *ZarfState) Collect(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("not implemented in %T", z)
}
