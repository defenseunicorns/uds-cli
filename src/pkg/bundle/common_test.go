// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
)

func Test_validateBundleVars(t *testing.T) {
	tests := []struct {
		name        string
		description string
		packages    []types.Package
		wantErr     bool
	}{
		{
			name:        "ImportMatchesExport",
			description: "import matches export",
			packages: []types.Package{
				{Name: "foo", Exports: []types.BundleVariableExport{{Name: "foo"}}},
				{Name: "bar", Imports: []types.BundleVariableImport{{Name: "foo", Package: "foo"}}},
			},
			wantErr: false,
		},
		{
			name:        "ImportDoesntMatchExport",
			description: "error when import doesn't match export",
			packages: []types.Package{
				{Name: "foo", Exports: []types.BundleVariableExport{{Name: "foo"}}},
				{Name: "bar", Imports: []types.BundleVariableImport{{Name: "bar", Package: "foo"}}},
			},
			wantErr: true,
		},
		{
			name:        "FirstPkgHasImport",
			description: "error when first pkg has an import",
			packages: []types.Package{
				{Name: "foo", Imports: []types.BundleVariableImport{{Name: "foo", Package: "foo"}}},
			},
			wantErr: true,
		},
		{
			name:        "PackageNamesMustMatch",
			description: "error when package name doesn't match",
			packages: []types.Package{
				{Name: "foo", Exports: []types.BundleVariableExport{{Name: "foo"}}},
				{Name: "bar", Imports: []types.BundleVariableImport{{Name: "foo", Package: "baz"}}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBundleVars(tt.packages)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func Test_validateOverrides(t *testing.T) {
	tests := []struct {
		name          string
		description   string
		bundlePackage types.Package
		zarfPackage   v1alpha1.ZarfPackage
		wantErr       bool
	}{
		{
			name:        "validOverride",
			description: "Respective components and charts exist for override",
			bundlePackage: types.Package{
				Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"component": {"chart": {}}}},
			zarfPackage: v1alpha1.ZarfPackage{
				Components: []v1alpha1.ZarfComponent{
					{Name: "component", Charts: []v1alpha1.ZarfChart{{Name: "chart"}}},
				},
			},
			wantErr: false,
		},
		{
			name:        "validOverrideMultipleComponents",
			description: "Respective components and charts exist for override when multiple charts and components are present",
			bundlePackage: types.Package{
				Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"component-a": {"chart-1": {}}}},
			zarfPackage: v1alpha1.ZarfPackage{
				Components: []v1alpha1.ZarfComponent{
					{Name: "component-a", Charts: []v1alpha1.ZarfChart{{Name: "chart-1"}, {Name: "chart-2"}}},
					{Name: "component-b", Charts: []v1alpha1.ZarfChart{{Name: "chart-b"}}},
				},
			},
			wantErr: false,
		},
		{
			name:        "invalidComponentOverride",
			description: "Component does not exist for override",
			bundlePackage: types.Package{
				Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"hell-unleashed": {"chart": {}}}},
			zarfPackage: v1alpha1.ZarfPackage{
				Components: []v1alpha1.ZarfComponent{
					{Name: "hello-world", Charts: []v1alpha1.ZarfChart{{Name: "chart"}}},
				},
			},
			wantErr: true,
		},
		{
			name:        "invalidChartOverride",
			description: "Chart does not exist for override",
			bundlePackage: types.Package{
				Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"component": {"hell-unleashed": {}}}},
			zarfPackage: v1alpha1.ZarfPackage{
				Components: []v1alpha1.ZarfComponent{
					{Name: "component", Charts: []v1alpha1.ZarfChart{{Name: "hello-world"}}},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOverrides(tt.bundlePackage, tt.zarfPackage)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func Test_getPkgPath(t *testing.T) {
	tests := []struct {
		name   string
		pkg    types.Package
		arch   string
		srcDir string
		want   string
	}{
		{
			name:   "init full path",
			pkg:    types.Package{Name: "init", Ref: "0.0.1", Path: "../fake/path/custom-init.tar.zst"},
			arch:   "fake64",
			srcDir: "/mock/source",
			want:   "/mock/fake/path/custom-init.tar.zst",
		},
		{
			name:   "init directory only path",
			pkg:    types.Package{Name: "init", Ref: "0.0.1", Path: "../fake/path"},
			arch:   "fake64",
			srcDir: "/mock/source",
			want:   "/mock/fake/path/zarf-init-fake64-0.0.1.tar.zst",
		},
		{
			name:   "full path",
			pkg:    types.Package{Name: "nginx", Ref: "0.0.1", Path: "./fake/zarf-package-nginx-fake64-0.0.1.tar.zst"},
			arch:   "fake64",
			srcDir: "/mock/source",
			want:   "/mock/source/fake/zarf-package-nginx-fake64-0.0.1.tar.zst",
		},
		{
			name:   "directory only path",
			pkg:    types.Package{Name: "nginx", Ref: "0.0.1", Path: "fake"},
			arch:   "fake64",
			srcDir: "/mock/source",
			want:   "/mock/source/fake/zarf-package-nginx-fake64-0.0.1.tar.zst",
		},
		{
			name:   "absolute path",
			pkg:    types.Package{Name: "nginx", Ref: "0.0.1", Path: "/fake/zarf-package-nginx-fake64-0.0.1.tar.zst"},
			arch:   "fake64",
			srcDir: "/mock/source",
			want:   "/fake/zarf-package-nginx-fake64-0.0.1.tar.zst",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := getPkgPath(tt.pkg, tt.arch, tt.srcDir)
			require.NoError(t, err)
			require.Equal(t, tt.want, path)
		})
	}
}
