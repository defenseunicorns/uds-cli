// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
)

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "values-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp YAML file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("failed to write temp YAML file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close temp YAML file: %v", err)
	}
	return f.Name()
}

func TestResolveValuesFiles(t *testing.T) {
	tests := []struct {
		name      string
		files     []string
		bundleDir string
		want      []string
	}{
		{
			name:      "all relative paths",
			files:     []string{"values/a.yaml", "values/b.yaml"},
			bundleDir: "/bundle",
			want:      []string{"/bundle/values/a.yaml", "/bundle/values/b.yaml"},
		},
		{
			name:      "all absolute paths unchanged",
			files:     []string{"/abs/a.yaml", "/abs/b.yaml"},
			bundleDir: "/bundle",
			want:      []string{"/abs/a.yaml", "/abs/b.yaml"},
		},
		{
			name:      "mixed relative and absolute",
			files:     []string{"rel.yaml", "/abs.yaml"},
			bundleDir: "/bundle",
			want:      []string{"/bundle/rel.yaml", "/abs.yaml"},
		},
		{
			name:      "empty file list",
			files:     []string{},
			bundleDir: "/bundle",
			want:      []string{},
		},
		{
			name:      "nil file list",
			files:     nil,
			bundleDir: "/bundle",
			want:      nil,
		},
		{
			name:      "dot-relative path cleaned",
			files:     []string{"./values/a.yaml"},
			bundleDir: "/bundle",
			want:      []string{"/bundle/values/a.yaml"},
		},
		{
			name:      "parent traversal resolved",
			files:     []string{"../sibling/a.yaml"},
			bundleDir: "/bundle/sub",
			want:      []string{"/bundle/sibling/a.yaml"},
		},
		{
			name:      "empty bundleDir leaves relative path as-is",
			files:     []string{"a.yaml"},
			bundleDir: "",
			want:      []string{"a.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveValuesFiles(tt.files, tt.bundleDir)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTemplateValuesFiles(t *testing.T) {
	tests := []struct {
		name         string
		fileContents []string // one entry per file
		vars         Variables
		wantErr      string
		wantOutputs  []string // expected rendered content per file
		wantSameRef  bool     // true when nil vars → paths must equal input paths
	}{
		{
			name:         "nil vars returns original paths",
			fileContents: []string{"host: static-value"},
			vars:         nil,
			wantOutputs:  []string{"host: static-value"},
			wantSameRef:  true,
		},
		{
			name:         "top-level string variable",
			fileContents: []string{"host: {{ .vars.domain }}"},
			vars:         Variables{"domain": "uds.dev"},
			wantOutputs:  []string{"host: uds.dev"},
		},
		{
			name:         "nested variable access",
			fileContents: []string{"enabled: {{ .vars.logging.enabled }}"},
			vars:         Variables{"logging": map[string]any{"enabled": true}},
			wantOutputs:  []string{"enabled: true"},
		},
		{
			name:         "numeric variable rendered as string",
			fileContents: []string{"replicas: {{ .vars.replicas }}"},
			vars:         Variables{"replicas": float64(3)},
			wantOutputs:  []string{"replicas: 3"},
		},
		{
			name:         "bool variable rendered as string",
			fileContents: []string{"enabled: {{ .vars.flag }}"},
			vars:         Variables{"flag": true},
			wantOutputs:  []string{"enabled: true"},
		},
		{
			name:         "no markers with non-nil vars",
			fileContents: []string{"static: value\nother: key"},
			vars:         Variables{"k": "v"},
			wantOutputs:  []string{"static: value\nother: key"},
		},
		{
			name:         "empty file",
			fileContents: []string{""},
			vars:         Variables{"k": "v"},
			wantOutputs:  []string{""},
		},
		{
			name:         "multiple files all rendered",
			fileContents: []string{"a: {{ .vars.x }}", "b: {{ .vars.x }}"},
			vars:         Variables{"x": "hello"},
			wantOutputs:  []string{"a: hello", "b: hello"},
		},
		{
			name:         "second file has missing variable",
			fileContents: []string{"a: {{ .vars.x }}", "b: {{ .vars.missing }}"},
			vars:         Variables{"x": "ok"},
			wantErr:      "map has no entry for key",
		},
		{
			name:         "missing variable key",
			fileContents: []string{"host: {{ .vars.undefined }}"},
			vars:         Variables{},
			wantErr:      "map has no entry for key",
		},
		{
			name:         "missing nested key",
			fileContents: []string{"x: {{ .vars.a.missing }}"},
			vars:         Variables{"a": map[string]any{"present": "yes"}},
			wantErr:      "map has no entry for key",
		},
		{
			name:         "invalid template syntax",
			fileContents: []string{"x: {{ .vars. }}"},
			vars:         Variables{"k": "v"},
			wantErr:      "", // any parse error
		},
		{
			name:         "empty vars map with marker",
			fileContents: []string{"x: {{ .vars.k }}"},
			vars:         Variables{},
			wantErr:      "map has no entry for key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var inputPaths []string
			for _, content := range tt.fileContents {
				inputPaths = append(inputPaths, writeTempYAML(t, content))
			}

			outPaths, err := templateValuesFiles(context.Background(), inputPaths, tt.vars, t.TempDir())

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			if !tt.wantSameRef && tt.wantOutputs == nil {
				// error case with empty wantErr: any error is acceptable
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, outPaths, len(tt.fileContents))

			if tt.wantSameRef {
				assert.Equal(t, inputPaths, outPaths)
				return
			}

			for i, wantContent := range tt.wantOutputs {
				assert.True(t, strings.HasSuffix(outPaths[i], ".yaml"),
					"temp file must have .yaml extension, got: %s", outPaths[i])
				got, err := os.ReadFile(outPaths[i])
				require.NoError(t, err)
				assert.Equal(t, wantContent, string(got))
			}
		})
	}
}

func TestZarfDeployer_DeployPackage_InvalidSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "nonexistent local tarball",
			source: "/path/to/package.tar.zst",
		},
		{
			name:   "empty source",
			source: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			deployer := NewZarfDeployer(out)

			pkg := &Package{
				Name:   "test-pkg",
				Source: tt.source,
			}

			err := deployer.DeployPackage(context.Background(), pkg, DeployPackageOptions{
				Config: &UDSBundleConfig{Global: &GlobalOptions{}, Options: &ConfigOptions{}},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to load package")
		})
	}
}

func TestNewZarfDeployer(t *testing.T) {
	out := &bytes.Buffer{}

	deployer := NewZarfDeployer(out)

	assert.NotNil(t, deployer)
	assert.NotNil(t, deployer.Out)
	// Out is wrapped in a syncWriter for concurrent safety; verify writes still reach the buffer.
	_, _ = fmt.Fprint(deployer.Out, "hello")
	assert.Equal(t, "hello", out.String())
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
