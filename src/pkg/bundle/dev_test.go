// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
)

func Test_handleExcludedComponents(t *testing.T) {
	tests := []struct {
		name              string
		b                 Bundle
		excludeComponents []string
		expected          []types.Package
		wantErr           bool
	}{
		{
			name:              "exclude components using duplicated pkg names",
			excludeComponents: []string{"pkg1.foo", "pkg1.bar"},
			b: Bundle{
				bundle: types.UDSBundle{
					Packages: []types.Package{
						{Name: "pkg1", OptionalComponents: []string{"foo", "bar", "baz"}},
					},
				},
			},
			expected: []types.Package{
				{Name: "pkg1", OptionalComponents: []string{"baz"}},
			},
		},
		{
			name:              "exclude all optional components from pkg",
			excludeComponents: []string{"pkg1.foo", "pkg1.bar"},
			b: Bundle{
				bundle: types.UDSBundle{
					Packages: []types.Package{
						{Name: "pkg1", OptionalComponents: []string{"foo", "bar"}},
					},
				},
			},
			expected: []types.Package{
				{Name: "pkg1", OptionalComponents: nil},
			},
		},
		{
			name:              "exclude components from multiple pkgs",
			excludeComponents: []string{"pkg1.foo", "pkg1.bar", "pkg2.foo", "pkg2.baz"},
			b: Bundle{
				bundle: types.UDSBundle{
					Packages: []types.Package{
						{Name: "pkg1", OptionalComponents: []string{"foo", "bar", "baz"}},
						{Name: "pkg2", OptionalComponents: []string{"foo", "bar", "baz"}},
					},
				},
			},
			expected: []types.Package{
				{Name: "pkg1", OptionalComponents: []string{"baz"}},
				{Name: "pkg2", OptionalComponents: []string{"bar"}},
			},
		},
		{
			name:              "err for invalid exclude syntax",
			excludeComponents: []string{"pkg1=foo", "pkg1.bar"},
			b: Bundle{
				bundle: types.UDSBundle{
					Packages: []types.Package{
						{Name: "pkg1", OptionalComponents: []string{"foo", "bar", "baz"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name:              "err when pkg doesn't exist in bundle",
			excludeComponents: []string{"pkg1.foo", "pkg2.bar"},
			b: Bundle{
				bundle: types.UDSBundle{
					Packages: []types.Package{
						{Name: "pkg1", OptionalComponents: []string{"foo"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name:              "err when component doesn't exist in pkg",
			excludeComponents: []string{"pkg1.foo", "pkg1.bar"},
			b: Bundle{
				bundle: types.UDSBundle{
					Packages: []types.Package{
						{Name: "pkg1", OptionalComponents: []string{"foo"}},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.b.handleExcludedComponents(tt.excludeComponents)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, tt.b.bundle.Packages)
		})
	}
}
