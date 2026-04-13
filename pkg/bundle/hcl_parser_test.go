// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBundleFile_SpecCompliant(t *testing.T) {
	path := filepath.Join("..", "..", "tests", "test_data", "bundles", "spec-compliant", "bundle.uds.hcl")

	b, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.NoError(t, err)

	// Metadata assertions
	assert.Equal(t, "uds.dev/v1alpha1", b.UDS.BundleAPIVersion)
	assert.Equal(t, "uds-core-example", b.Metadata.Name)
	assert.Equal(t, "A spec-compliant UDS Core bundle example with base, logging, and monitoring packages", b.Metadata.Description)
	assert.Equal(t, "0.1.0", b.Metadata.Version)

	// Package count
	require.Len(t, b.Packages, 3)

	// core_base: locals fully resolved — verify structure, not pinned version
	assert.Equal(t, "core_base", b.Packages[0].Name)
	wantSourcePrefix := "oci://ghcr.io/defenseunicorns/packages/uds/core-base:"
	assert.True(t, strings.HasPrefix(b.Packages[0].Source, wantSourcePrefix),
		"expected source to start with %q, got %q", wantSourcePrefix, b.Packages[0].Source)
	// Ensure the version suffix was resolved (not left as a template)
	version := strings.TrimPrefix(b.Packages[0].Source, wantSourcePrefix)
	assert.NotEmpty(t, version, "version tag should not be empty")
	assert.NotContains(t, version, "${", "version should not contain unresolved template expressions")
	assert.Equal(t, []string{"istio-passthrough-gateway", "istio-egress-gateway"}, b.Packages[0].OptionalComponents)

	// core_logging: depends_on and valuesFiles
	assert.Equal(t, "core_logging", b.Packages[1].Name)
	require.Len(t, b.Packages[1].DependsOn, 1)
	assert.Equal(t, "core_base", b.Packages[1].DependsOn[0].Name)
	assert.Equal(t, []string{"values/loki.yaml", "values/vector.yaml"}, b.Packages[1].ValuesFiles)

	// core_monitoring: namespace, depends_on with 2 entries
	assert.Equal(t, "monitoring", b.Packages[2].Namespace)
	require.Len(t, b.Packages[2].DependsOn, 2)
	depNames := []string{b.Packages[2].DependsOn[0].Name, b.Packages[2].DependsOn[1].Name}
	assert.Contains(t, depNames, "core_base")
	assert.Contains(t, depNames, "core_logging")
	assert.Equal(t, []string{"values/monitoring.yaml"}, b.Packages[2].ValuesFiles)
}

func TestParseBundleFile_MinimalBundle(t *testing.T) {
	hcl := `
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "minimal" }
package "pkg1" { source = "oci://example.com/pkg:v1" }
`
	path := writeTempHCL(t, hcl)

	b, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, "minimal", b.Metadata.Name)
	require.Len(t, b.Packages, 1)
	assert.Equal(t, "pkg1", b.Packages[0].Name)
}

func TestParseBundleFile_NoLocals(t *testing.T) {
	hcl := `
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "no-locals" }
package "app" { source = "oci://example.com/app:v2" }
`
	path := writeTempHCL(t, hcl)

	b, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, "oci://example.com/app:v2", b.Packages[0].Source)
}

func TestParseBundleFile_NestedLocals(t *testing.T) {
	hcl := `
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "nested-locals" }
locals {
  registry = "ghcr.io/myorg"
  pkgs = {
    app = "my-app"
  }
  ver = "1.0.0"
}
package "app" { source = "oci://${local.registry}/${local.pkgs.app}:${local.ver}" }
`
	path := writeTempHCL(t, hcl)

	b, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, "oci://ghcr.io/myorg/my-app:1.0.0", b.Packages[0].Source)
}

func TestParseBundleFile_OptionalFields(t *testing.T) {
	hcl := `
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "optional-test" }
package "pkg1" { source = "oci://example.com/pkg:v1" }
`
	path := writeTempHCL(t, hcl)

	b, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.NoError(t, err)
	assert.Empty(t, b.Metadata.Description)
	assert.Empty(t, b.Metadata.Version)
	assert.Empty(t, b.Packages[0].Namespace)
	assert.Empty(t, b.Packages[0].DependsOn)
	assert.Empty(t, b.Packages[0].ValuesFiles)
	assert.Empty(t, b.Packages[0].OptionalComponents)
}

func TestParseBundleFile_OptionalComponents(t *testing.T) {
	hcl := `
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "optional-components-test" }
package "core" {
  source = "oci://example.com/core:v1"
  optional_components = [
    "istio-passthrough-gateway",
    "istio-egress-gateway",
  ]
}
`
	path := writeTempHCL(t, hcl)

	b, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.NoError(t, err)
	require.Len(t, b.Packages, 1)
	assert.Equal(t, []string{"istio-passthrough-gateway", "istio-egress-gateway"}, b.Packages[0].OptionalComponents)
}

func TestParseBundleFile_EmptyOptionalComponents(t *testing.T) {
	hcl := `
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "empty-optional-components" }
package "core" {
  source = "oci://example.com/core:v1"
  optional_components = []
}
`
	path := writeTempHCL(t, hcl)

	b, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.NoError(t, err)
	assert.Empty(t, b.Packages[0].OptionalComponents)
}

func TestParseBundleFile_FileNotFound(t *testing.T) {
	_, err := NewHCLParser().ParseBundleFile(context.Background(), "/nonexistent/bundle.uds.hcl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read bundle file")
}

func TestParseBundleFile_InvalidHCL(t *testing.T) {
	path := writeTempHCL(t, "this is not valid HCL {{{}}")

	_, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.Error(t, err)
}

func TestParseBundleFile_MissingAPIVersion(t *testing.T) {
	hcl := `
uds { }
metadata { name = "test" }
package "pkg1" { source = "oci://example.com/pkg:v1" }
`
	path := writeTempHCL(t, hcl)

	// HCL enforces bundle_api_version as required at decode time
	_, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bundle_api_version")
}

func TestParseBundleFile_MissingMetadataName(t *testing.T) {
	hcl := `
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { }
package "pkg1" { source = "oci://example.com/pkg:v1" }
`
	path := writeTempHCL(t, hcl)

	_, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.Error(t, err)
	// HCL enforces "name" as required at parse time since it has no optional tag
	assert.Contains(t, err.Error(), "name")
}

func TestParseBundleFile_MissingPackageSource(t *testing.T) {
	hcl := `
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test" }
package "pkg1" { }
`
	path := writeTempHCL(t, hcl)

	_, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.Error(t, err)
}

func TestParseBundleFile_DependsOnStringLiteral(t *testing.T) {
	// Test that string literals in depends_on are rejected (must use expression syntax)
	hcl := `
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test" }
package "pkg1" { source = "oci://example.com/pkg:v1" }
package "pkg2" {
  source     = "oci://example.com/pkg:v2"
  depends_on = ["pkg1"]
}
`
	path := writeTempHCL(t, hcl)

	_, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package reference")
}

func TestParseBundleFile_DependsOnWrongRootName(t *testing.T) {
	// Test that traversals with wrong root name are rejected (must start with 'package')
	hcl := `
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test" }
package "pkg1" { source = "oci://example.com/pkg:v1" }
package "pkg2" {
  source     = "oci://example.com/pkg:v2"
  depends_on = [module.pkg1]
}
`
	path := writeTempHCL(t, hcl)

	_, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must start with 'package'")
}

func TestParseBundleFile_DependsOnTooManyParts(t *testing.T) {
	// Test that traversals with too many parts are rejected (must be package.<name>)
	hcl := `
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test" }
package "pkg1" { source = "oci://example.com/pkg:v1" }
package "pkg2" {
  source     = "oci://example.com/pkg:v2"
  depends_on = [package.pkg1.extra]
}
`
	path := writeTempHCL(t, hcl)

	_, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected package.<name>")
}

func TestParseBundleFile_InvalidDependsOn(t *testing.T) {
	hcl := `
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = "test" }
package "pkg1" { source = "oci://example.com/pkg:v1" }
package "pkg2" {
  source     = "oci://example.com/pkg:v2"
  depends_on = [package.nonexistent]
}
`
	path := writeTempHCL(t, hcl)

	// Parses successfully — validation is the caller's responsibility
	b, err := NewHCLParser().ParseBundleFile(context.Background(), path)
	require.NoError(t, err)

	err = b.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown package")
}

func TestParseBundleBytes_ValidHCL(t *testing.T) {
	src := []byte(`
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata {
  name    = "my-bundle"
  version = "1.0.0"
}
package "pkg1" { source = "oci://example.com/pkg:v1" }
`)
	b, err := NewHCLParser().ParseBundleBytes(context.Background(), src)
	require.NoError(t, err)
	assert.Equal(t, "my-bundle", b.Metadata.Name)
	assert.Equal(t, "1.0.0", b.Metadata.Version)
}

func TestParseBundleBytes_InvalidHCL(t *testing.T) {
	_, err := NewHCLParser().ParseBundleBytes(context.Background(), []byte("this is not valid HCL {{{"))
	require.ErrorContains(t, err, "failed to parse HCL")
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		bundle  UDSBundle
		wantErr string
	}{
		{
			name: "valid bundle",
			bundle: UDSBundle{
				UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
				Metadata: Metadata{Name: "test"},
				Packages: []Package{{Name: "pkg1", Source: "oci://example.com/pkg:v1"}},
			},
			wantErr: "",
		},
		{
			name: "missing api version",
			bundle: UDSBundle{
				Metadata: Metadata{Name: "test"},
				Packages: []Package{{Name: "pkg1", Source: "oci://example.com/pkg:v1"}},
			},
			wantErr: "bundle_api_version",
		},
		{
			name: "unsupported api version",
			bundle: UDSBundle{
				UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v2beta1"},
				Metadata: Metadata{Name: "test"},
				Packages: []Package{{Name: "pkg1", Source: "oci://example.com/pkg:v1"}},
			},
			wantErr: "not supported",
		},
		{
			name: "missing name",
			bundle: UDSBundle{
				UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
				Packages: []Package{{Name: "pkg1", Source: "oci://example.com/pkg:v1"}},
			},
			wantErr: "metadata.name",
		},
		{
			name: "duplicate package names",
			bundle: UDSBundle{
				UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
				Metadata: Metadata{Name: "test"},
				Packages: []Package{
					{Name: "pkg1", Source: "oci://a:v1"},
					{Name: "pkg1", Source: "oci://b:v1"},
				},
			},
			wantErr: "duplicate package name",
		},
		{
			name: "missing package source",
			bundle: UDSBundle{
				UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
				Metadata: Metadata{Name: "test"},
				Packages: []Package{{Name: "pkg1"}},
			},
			wantErr: "source is required",
		},
		{
			name: "unknown depends_on",
			bundle: UDSBundle{
				UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
				Metadata: Metadata{Name: "test"},
				Packages: []Package{
					{Name: "pkg1", Source: "oci://a:v1", DependsOn: []PackageRef{{Name: "missing"}}},
				},
			},
			wantErr: "unknown package",
		},
		{
			name: "no packages",
			bundle: UDSBundle{
				UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
				Metadata: Metadata{Name: "test"},
				Packages: []Package{},
			},
			wantErr: "at least one package is required",
		},
		{
			name: "self-referencing dependency",
			bundle: UDSBundle{
				UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
				Metadata: Metadata{Name: "test"},
				Packages: []Package{
					{Name: "pkg1", Source: "oci://a:v1", DependsOn: []PackageRef{{Name: "pkg1"}}},
				},
			},
			wantErr: "cannot depend on itself",
		},
		{
			name: "duplicate optional component",
			bundle: UDSBundle{
				UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
				Metadata: Metadata{Name: "test"},
				Packages: []Package{
					{Name: "pkg1", Source: "oci://a:v1", OptionalComponents: []string{"comp1", "comp1"}},
				},
			},
			wantErr: "duplicate optional component",
		},
		{
			name: "empty string in optional components",
			bundle: UDSBundle{
				UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
				Metadata: Metadata{Name: "test"},
				Packages: []Package{
					{Name: "pkg1", Source: "oci://a:v1", OptionalComponents: []string{"comp1", ""}},
				},
			},
			wantErr: "optional_components contains empty string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bundle.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func writeTempHCL(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.uds.hcl")
	if err := os.WriteFile(path, []byte(content), tmpFilePerm); err != nil {
		t.Fatalf("failed to write temp HCL file: %v", err)
	}
	return path
}

func TestParseDefaults(t *testing.T) {
	tests := []struct {
		name    string
		hcl     string
		fixture string // file path; takes precedence over hcl
		wantErr string // substring expected in error; empty means any error
		wantOK  bool
		check   func(t *testing.T, vars Variables)
	}{
		{
			name:   "valid with nested variables",
			wantOK: true,
			hcl: `variables = {
  domain = "example.com"
  replicas = 3
  nested = {
    enabled = true
  }
}`,
			check: func(t *testing.T, vars Variables) {
				assert.Equal(t, "example.com", vars["domain"])
				assert.InDelta(t, float64(3), vars["replicas"], 0.001)
				nested, ok := vars["nested"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, true, nested["enabled"])
			},
		},
		{
			name:   "empty file returns nil variables",
			wantOK: true,
			hcl:    "",
			check: func(t *testing.T, vars Variables) {
				assert.Nil(t, vars)
			},
		},
		{
			name:    "rejects block (options)",
			hcl:     `options { architecture = "amd64" }`,
			wantErr: "block",
		},
		{
			name:    "rejects block (arbitrary)",
			hcl:     `something { foo = "bar" }`,
			wantErr: "block",
		},
		{
			name:    "rejects unknown top-level attribute",
			hcl:     "variables = { a = \"b\" }\nunknown_key = \"bad\"",
			wantErr: "unknown_key",
		},
		{
			name:    "rejects malformed HCL",
			hcl:     `not valid hcl {{{`,
			wantErr: "",
		},
		{
			name:    "file not found",
			fixture: "/nonexistent/defaults.uds.hcl",
			wantErr: "cannot read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var path string
			if tt.fixture != "" {
				path = tt.fixture
			} else {
				path = filepath.Join(t.TempDir(), "defaults.uds.hcl")
				require.NoError(t, os.WriteFile(path, []byte(tt.hcl), tmpFilePerm))
			}

			vars, err := ParseDefaults(context.Background(), path)

			if !tt.wantOK {
				require.Error(t, err)
				if tt.wantErr != "" {
					assert.Contains(t, err.Error(), tt.wantErr)
				}
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, vars)
			}
		})
	}
}

func TestParseBundleConfig(t *testing.T) {
	specCompliantPath := filepath.Join("..", "..", "tests", "test_data",
		"bundles", "spec-compliant", "config.uds.hcl")

	tests := []struct {
		name    string
		hcl     string // written to a temp file; empty when fixture is used
		fixture string // path to a fixture file; takes precedence over hcl
		wantErr string // substring expected in error; empty means "any error is fine" for error cases
		wantOK  bool   // true means success expected; false with non-empty wantErr means error expected
		check   func(t *testing.T, cfg *UDSBundleConfig)
	}{
		// ---- fixture: all options + nested variables ----
		{
			name:    "spec-compliant full config",
			fixture: specCompliantPath,
			wantOK:  true,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				// Options (seven fields from ADR-0006)
				assert.Equal(t, "info", cfg.Options.LogLevel)
				assert.Equal(t, "amd64", cfg.Options.Architecture)
				assert.False(t, cfg.Options.PlainHTTP)
				assert.False(t, cfg.Options.SkipTLSVerify)
				assert.Equal(t, "/tmp/uds-cache", cfg.Options.UDSCache)
				assert.Equal(t, "/tmp/uds-tmp", cfg.Options.TmpDir)
				assert.Equal(t, 10, cfg.Options.Concurrency)
				// Top-level scalar variable
				assert.Equal(t, "uds.dev", cfg.Variables["domain"])
				// Nested object preserved as map[string]any
				logging, ok := cfg.Variables["logging"].(map[string]any)
				require.True(t, ok, "logging should decode to map[string]any")
				assert.True(t, logging["vectorEnabled"].(bool))
				assert.Equal(t, "collector", logging["vectorRole"])
				monitoring, ok := cfg.Variables["monitoring"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "15d", monitoring["retentionDays"])
			},
		},

		// ---- options block only, no variables ----
		{
			name:   "options only",
			wantOK: true,
			hcl: `
options {
  architecture = "arm64"
  concurrency  = 5
}`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				assert.Equal(t, "arm64", cfg.Options.Architecture)
				assert.Equal(t, 5, cfg.Options.Concurrency)
				assert.Nil(t, cfg.Variables)
			},
		},

		// ---- variables only, no options block ----
		{
			name:   "variables only",
			wantOK: true,
			hcl:    `variables = { domain = "example.com" }`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				assert.Nil(t, cfg.Options)
				assert.Equal(t, "example.com", cfg.Variables["domain"])
			},
		},

		// ---- empty file: no error, everything zero ----
		{
			name:   "empty file",
			wantOK: true,
			hcl:    ``,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				assert.Nil(t, cfg.Options)
				assert.Nil(t, cfg.Variables)
			},
		},

		// ---- all scalar types: string, number, bool ----
		{
			name:   "scalar types: string number bool",
			wantOK: true,
			hcl: `
variables = {
  str  = "hello"
  num  = 42
  flag = true
}`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				assert.Equal(t, "hello", cfg.Variables["str"])
				assert.InDelta(t, float64(42), cfg.Variables["num"], 0.001)
				assert.True(t, cfg.Variables["flag"].(bool))
			},
		},

		// ---- float number ----
		{
			name:   "float variable",
			wantOK: true,
			hcl:    `variables = { ratio = 1.5 }`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				assert.InEpsilon(t, float64(1.5), cfg.Variables["ratio"], 0.0001)
			},
		},

		// ---- empty variables object ----
		{
			name:   "empty variables object",
			wantOK: true,
			hcl:    `variables = {}`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				assert.Empty(t, cfg.Variables)
			},
		},

		// ---- deep nesting (3 levels) ----
		{
			name:   "three levels of nesting",
			wantOK: true,
			hcl: `
variables = {
  a = {
    b = {
      c = "deep"
    }
  }
}`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				a, ok := cfg.Variables["a"].(map[string]any)
				require.True(t, ok)
				b, ok := a["b"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "deep", b["c"])
			},
		},

		// ---- bool options fields ----
		{
			name:   "bool options",
			wantOK: true,
			hcl: `
options {
  plain_http      = true
  skip_tls_verify = true
}`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				assert.True(t, cfg.Options.PlainHTTP)
				assert.True(t, cfg.Options.SkipTLSVerify)
			},
		},

		// ---- concurrency zero value (explicit) ----
		{
			name:   "concurrency explicit zero",
			wantOK: true,
			hcl:    `options { concurrency = 0 }`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				assert.Equal(t, 0, cfg.Options.Concurrency)
			},
		},

		// ---- file not found ----
		{
			name:    "file not found",
			fixture: "/nonexistent/config.uds.hcl",
			wantErr: "cannot read",
		},

		// ---- malformed HCL (any error expected) ----
		{
			name:    "invalid HCL syntax",
			hcl:     `options { = "broken" }`,
			wantErr: "",
		},

		// ---- unknown option attribute (gohcl rejects it) ----
		{
			name:    "unknown option attribute",
			hcl:     `options { unknown_field = "x" }`,
			wantErr: "",
		},

		// ---- duplicate options block ----
		{
			name: "duplicate options blocks",
			hcl: `
options { architecture = "amd64" }
options { architecture = "arm64" }`,
			wantErr: "",
		},

		// ---- variables must be an object ----
		{
			name:    "variables is a string not an object",
			hcl:     `variables = "not-an-object"`,
			wantErr: "variables must be an object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path string
			if tt.fixture != "" {
				path = tt.fixture
			} else {
				path = writeTempHCL(t, tt.hcl)
			}

			cfg, err := NewHCLParser().ParseBundleConfig(context.Background(), path)

			// Error cases: either wantErr substring pinned, or any error is fine
			if !tt.wantOK {
				require.Error(t, err)
				if tt.wantErr != "" {
					assert.Contains(t, err.Error(), tt.wantErr)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cfg)
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}
