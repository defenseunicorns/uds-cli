// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
)

func TestZarfDeployer_DeployPackage_UnsupportedSource(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		wantErr string
	}{
		{
			name:    "local tarball not supported",
			source:  "/path/to/package.tar.zst",
			wantErr: "unsupported source type",
		},
		{
			name:    "http URL not supported",
			source:  "https://example.com/package.tar.zst",
			wantErr: "unsupported source type",
		},
		{
			name:    "empty source",
			source:  "",
			wantErr: "unsupported source type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			deployer := NewZarfDeployer(t.TempDir(), out)

			pkg := &Package{
				Name:   "test-pkg",
				Source: tt.source,
			}

			err := deployer.DeployPackage(context.Background(), pkg, DeployPackageOptions{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNewZarfDeployer(t *testing.T) {
	out := &bytes.Buffer{}
	tempDir := "/tmp/test"

	deployer := NewZarfDeployer(tempDir, out)

	assert.NotNil(t, deployer)
	assert.Equal(t, tempDir, deployer.TempDir)
	assert.Equal(t, out, deployer.Out)
}

func TestBuildComponentFilter(t *testing.T) {
	requiredTrue := true
	requiredFalse := false

	// Helper to extract component names from filter result
	componentNames := func(components []v1alpha1.ZarfComponent) []string {
		names := make([]string, len(components))
		for i, c := range components {
			names[i] = c.Name
		}
		return names
	}

	tests := []struct {
		name               string
		optionalComponents []string
		zarfComponents     []v1alpha1.ZarfComponent
		wantNames          []string
		wantErr            bool
	}{
		{
			name:               "empty list excludes optional components",
			optionalComponents: []string{},
			zarfComponents: []v1alpha1.ZarfComponent{
				{Name: "required-comp", Required: &requiredTrue},
				{Name: "default-comp", Default: true},
				{Name: "optional-comp"},
			},
			wantNames: []string{"required-comp", "default-comp"},
		},
		{
			name:               "nil list excludes optional components",
			optionalComponents: nil,
			zarfComponents: []v1alpha1.ZarfComponent{
				{Name: "required-comp", Required: &requiredTrue},
				{Name: "optional-comp"},
			},
			wantNames: []string{"required-comp"},
		},
		{
			name:               "explicit include adds optional component",
			optionalComponents: []string{"optional-comp"},
			zarfComponents: []v1alpha1.ZarfComponent{
				{Name: "required-comp", Required: &requiredTrue},
				{Name: "optional-comp"},
			},
			wantNames: []string{"required-comp", "optional-comp"},
		},
		{
			name:               "dash prefix excludes a default component",
			optionalComponents: []string{"-default-comp"},
			zarfComponents: []v1alpha1.ZarfComponent{
				{Name: "required-comp", Required: &requiredTrue},
				{Name: "default-comp", Default: true},
			},
			wantNames: []string{"required-comp"},
		},
		{
			name:               "required components always included regardless of exclusion",
			optionalComponents: []string{"-required-comp"},
			zarfComponents: []v1alpha1.ZarfComponent{
				{Name: "required-comp", Required: &requiredTrue},
			},
			wantNames: []string{"required-comp"},
		},
		{
			name:               "mix of includes and excludes",
			optionalComponents: []string{"opt-a", "-opt-b"},
			zarfComponents: []v1alpha1.ZarfComponent{
				{Name: "required-comp", Required: &requiredTrue},
				{Name: "opt-a"},
				{Name: "opt-b", Default: true},
			},
			wantNames: []string{"required-comp", "opt-a"},
		},
		{
			name:               "explicitly-false required treated as optional",
			optionalComponents: []string{},
			zarfComponents: []v1alpha1.ZarfComponent{
				{Name: "required-comp", Required: &requiredTrue},
				{Name: "not-required-comp", Required: &requiredFalse},
			},
			wantNames: []string{"required-comp"},
		},
		{
			name:               "all required components included with empty list",
			optionalComponents: []string{},
			zarfComponents: []v1alpha1.ZarfComponent{
				{Name: "req-a", Required: &requiredTrue},
				{Name: "req-b", Required: &requiredTrue},
				{Name: "opt-a"},
			},
			wantNames: []string{"req-a", "req-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := BuildComponentFilter(tt.optionalComponents)

			pkg := v1alpha1.ZarfPackage{
				Components: tt.zarfComponents,
			}

			selected, err := filter.Apply(pkg)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantNames, componentNames(selected))
		})
	}
}
