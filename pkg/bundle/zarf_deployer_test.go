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

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
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
			vars:         Variables{"logging": Variables{"enabled": true}},
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
			vars:         Variables{"a": Variables{"present": "yes"}},
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
		{
			name: "range over list of objects",
			fileContents: []string{
				"service:\n  ports:\n{{- range .vars.ports }}\n    - name: {{ .name }}\n      port: {{ .port }}\n{{- end }}",
			},
			vars: Variables{"ports": []any{
				Variables{"name": "a", "port": float64(80)},
				Variables{"name": "b", "port": float64(90)},
			}},
			wantOutputs: []string{
				"service:\n  ports:\n    - name: a\n      port: 80\n    - name: b\n      port: 90",
			},
		},
		{
			name: "range over list of objects with nested map",
			fileContents: []string{
				"keycloak:\n  extraVolumes:\n{{- range .vars.vols }}\n    - name: {{ .name }}\n      secret:\n        secretName: {{ .secret.secretName }}\n{{- end }}",
			},
			vars: Variables{"vols": []any{
				Variables{"name": "tls-certs", "secret": Variables{"secretName": "kc-tls"}},
			}},
			wantOutputs: []string{
				"keycloak:\n  extraVolumes:\n    - name: tls-certs\n      secret:\n        secretName: kc-tls",
			},
		},
		{
			name:         "if guard around optional list",
			fileContents: []string{"obj:\n{{- if .vars.tags }}\n  tags:\n{{- range .vars.tags }}\n    - {{ . }}\n{{- end }}\n{{- end }}"},
			vars:         Variables{"tags": []any{"prod", "logs"}},
			wantOutputs:  []string{"obj:\n  tags:\n    - prod\n    - logs"},
		},
		{
			name:         "with scope over nested map",
			fileContents: []string{"{{- with .vars.db }}host: {{ .host }}\nport: {{ .port }}{{- end }}"},
			vars:         Variables{"db": Variables{"host": "localhost", "port": float64(5432)}},
			wantOutputs:  []string{"host: localhost\nport: 5432"},
		},
		{
			name:         "env helper is undefined",
			fileContents: []string{"x: {{ env \"PATH\" }}"},
			vars:         Variables{"k": "v"},
			wantErr:      "function \"env\" not defined",
		},
		{
			name:         "default helper is undefined",
			fileContents: []string{"x: {{ default \"fallback\" .vars.present }}"},
			vars:         Variables{"present": "yes"},
			wantErr:      "function \"default\" not defined",
		},
		{
			name:         "toYaml helper is undefined",
			fileContents: []string{"x: {{ toYaml .vars.x }}"},
			vars:         Variables{"x": "y"},
			wantErr:      "function \"toYaml\" not defined",
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
				require.ErrorContains(t, err, tt.wantErr)
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

func TestPrepareValuesAndVariables(t *testing.T) {
	t.Run("scalars produce flattened SetVariables", func(t *testing.T) {
		dir := t.TempDir()
		valuesPath := writeTempYAML(t, "host: {{ .vars.domain }}")

		d := NewZarfDeployer(iostreams.IOStreams{}, nil)
		pkg := &Package{Name: "p", ValuesFiles: []string{valuesPath}}
		opts := DeployPackageOptions{
			Config: &UDSBundleConfig{
				Options:   &ConfigOptions{TmpDir: dir},
				Variables: Variables{"domain": "uds.dev"},
			},
		}

		_, setVars, err := d.prepareValuesAndVariables(context.Background(), iostreams.IOStreams{}, pkg, opts)
		require.NoError(t, err)
		assert.Equal(t, "uds.dev", setVars["DOMAIN"])
	})

	t.Run("collection variable skipped from SetVariables, passed via values file", func(t *testing.T) {
		dir := t.TempDir()
		// values file uses range so it can iterate over the nested list variable
		valuesPath := writeTempYAML(t,
			"ports:\n{{- range .vars.ports }}\n  - {{ .name }}\n{{- end }}",
		)

		d := NewZarfDeployer(iostreams.IOStreams{}, nil)
		pkg := &Package{Name: "p", ValuesFiles: []string{valuesPath}}
		opts := DeployPackageOptions{
			Config: &UDSBundleConfig{
				Options: &ConfigOptions{TmpDir: dir},
				Variables: Variables{"ports": []any{
					Variables{"name": "a"},
					Variables{"name": "b"},
				}},
			},
		}

		_, setVars, err := d.prepareValuesAndVariables(context.Background(), iostreams.IOStreams{}, pkg, opts)
		require.NoError(t, err)
		// Complex types are skipped from Flatten and must flow through values_files
		assert.NotContains(t, setVars, "PORTS")
	})

	t.Run("no values files, only variables", func(t *testing.T) {
		d := NewZarfDeployer(iostreams.IOStreams{}, nil)
		pkg := &Package{Name: "p"}
		opts := DeployPackageOptions{
			Config: &UDSBundleConfig{
				Options:   &ConfigOptions{},
				Variables: Variables{"x": "y"},
			},
		}

		zv, setVars, err := d.prepareValuesAndVariables(context.Background(), iostreams.IOStreams{}, pkg, opts)
		require.NoError(t, err)
		assert.Nil(t, zv)
		assert.Equal(t, "y", setVars["X"])
	})

	t.Run("non-scalar variables silently skipped in Flatten", func(t *testing.T) {
		d := NewZarfDeployer(iostreams.IOStreams{}, nil)
		pkg := &Package{Name: "p"}
		opts := DeployPackageOptions{
			Config: &UDSBundleConfig{
				Options: &ConfigOptions{},
				// chan is a non-scalar, synthetic type; never produced by HCL parser
				// Flatten silently skips non-scalars instead of erroring
				Variables: Variables{"k": []any{make(chan int)}},
			},
		}

		_, setVars, err := d.prepareValuesAndVariables(context.Background(), iostreams.IOStreams{}, pkg, opts)
		require.NoError(t, err)
		// Non-scalar "k" is omitted from setVars
		assert.NotContains(t, setVars, "K")
	})

	t.Run("template error wraps with package name", func(t *testing.T) {
		dir := t.TempDir()
		valuesPath := writeTempYAML(t, "x: {{ .vars.missing }}")

		d := NewZarfDeployer(iostreams.IOStreams{}, nil)
		pkg := &Package{Name: "broken", ValuesFiles: []string{valuesPath}}
		opts := DeployPackageOptions{
			Config: &UDSBundleConfig{
				Options:   &ConfigOptions{TmpDir: dir},
				Variables: Variables{"present": "yes"},
			},
		}

		_, _, err := d.prepareValuesAndVariables(context.Background(), iostreams.IOStreams{}, pkg, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"broken"`)
		assert.Contains(t, err.Error(), "failed to template")
	})
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
			deployer := NewZarfDeployer(iostreams.New(nil, nil, out), nil)

			pkg := &Package{
				Name:   "test-pkg",
				Source: tt.source,
			}

			err := deployer.DeployPackage(context.Background(), pkg, DeployPackageOptions{
				Config:    newTestConfig(),
				BundleDir: t.TempDir(),
			})
			require.ErrorContains(t, err, "failed to load package")
		})
	}
}

func TestNewZarfDeployer(t *testing.T) {
	errOut := &bytes.Buffer{}

	deployer := NewZarfDeployer(iostreams.New(nil, nil, errOut), nil)

	assert.NotNil(t, deployer)
	assert.NotNil(t, deployer.streams.ErrOut())
	// ErrOut() returns the synchronized writer; verify writes reach the underlying buffer.
	_, _ = fmt.Fprint(deployer.streams.ErrOut(), "hello")
	assert.Equal(t, "hello", errOut.String())
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
