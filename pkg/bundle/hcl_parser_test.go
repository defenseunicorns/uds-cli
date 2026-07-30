// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestParseBundleFile_SpecCompliant(t *testing.T) {
	path := filepath.Join("..", "..", "tests", "test_data", "bundles", "spec-compliant", "bundle.uds.hcl")

	b, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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

	b, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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

	b, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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

	b, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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

	b, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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

	b, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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

	b, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
	require.NoError(t, err)
	assert.Empty(t, b.Packages[0].OptionalComponents)
}

func TestParseBundleFile_FileNotFound(t *testing.T) {
	_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), "/nonexistent/bundle.uds.hcl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read bundle file")
}

func TestParseBundleFile_InvalidHCL(t *testing.T) {
	path := writeTempHCL(t, "this is not valid HCL {{{}}")

	_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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
	_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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

	_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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

	_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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

	_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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

	_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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

	_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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
	b, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(t.Context(), path)
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
	b, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleBytes(t.Context(), src)
	require.NoError(t, err)
	assert.Equal(t, "my-bundle", b.Metadata.Name)
	assert.Equal(t, "1.0.0", b.Metadata.Version)
}

func TestParseBundleBytes_InvalidHCL(t *testing.T) {
	_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleBytes(t.Context(), []byte("this is not valid HCL {{{"))
	require.ErrorContains(t, err, "failed to parse HCL")
}

func TestParseBundleBytes_SysArch(t *testing.T) {
	src := []byte(`
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata {
  name    = "arch-test"
  version = "1.0.0"
}
package "pkg" { source = "./pkg-${sys.arch}-1.0.0.tar.zst" }
`)

	t.Run("explicit arch wins", func(t *testing.T) {
		b, err := NewHCLParser("amd64", iostreams.IOStreams{}).ParseBundleBytes(t.Context(), src)
		require.NoError(t, err)
		assert.Equal(t, "./pkg-amd64-1.0.0.tar.zst", b.Packages[0].Source)
	})

	t.Run("empty arch falls back to runtime.GOARCH", func(t *testing.T) {
		b, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleBytes(t.Context(), src)
		require.NoError(t, err)
		assert.Equal(t, "./pkg-"+runtime.GOARCH+"-1.0.0.tar.zst", b.Packages[0].Source)
	})
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

// TestCtyValueToGo exercises every type branch of ctyValueToGo directly via
// synthesized cty.Value instances, bypassing the HCL parse layer. This makes
// branch coverage and error-path assertions unambiguous.
func TestCtyValueToGo(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		got, err := ctyValueToGo(cty.StringVal("x"))
		require.NoError(t, err)
		assert.Equal(t, "x", got)
	})

	t.Run("number", func(t *testing.T) {
		got, err := ctyValueToGo(cty.NumberIntVal(7))
		require.NoError(t, err)
		assert.InDelta(t, float64(7), got, 0.001)
	})

	t.Run("bool", func(t *testing.T) {
		got, err := ctyValueToGo(cty.BoolVal(true))
		require.NoError(t, err)
		assert.Equal(t, true, got)
	})

	t.Run("null is rejected", func(t *testing.T) {
		_, err := ctyValueToGo(cty.NullVal(cty.String))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "null values are not supported")
	})

	t.Run("unknown is rejected", func(t *testing.T) {
		_, err := ctyValueToGo(cty.UnknownVal(cty.String))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown values are not supported")
	})

	t.Run("list", func(t *testing.T) {
		got, err := ctyValueToGo(cty.ListVal([]cty.Value{
			cty.StringVal("a"),
			cty.StringVal("b"),
		}))
		require.NoError(t, err)
		assert.Equal(t, []any{"a", "b"}, got)
	})

	t.Run("empty list non-nil", func(t *testing.T) {
		got, err := ctyValueToGo(cty.ListValEmpty(cty.String))
		require.NoError(t, err)
		s, ok := got.([]any)
		require.True(t, ok, "expected []any, got %T", got)
		assert.NotNil(t, s)
		assert.Empty(t, s)
	})

	t.Run("set deduplicates", func(t *testing.T) {
		got, err := ctyValueToGo(cty.SetVal([]cty.Value{
			cty.NumberIntVal(1),
			cty.NumberIntVal(1),
			cty.NumberIntVal(2),
		}))
		require.NoError(t, err)
		s, ok := got.([]any)
		require.True(t, ok)
		assert.Len(t, s, 2)
	})

	t.Run("tuple heterogeneous", func(t *testing.T) {
		got, err := ctyValueToGo(cty.TupleVal([]cty.Value{
			cty.NumberIntVal(1),
			cty.StringVal("two"),
			cty.BoolVal(true),
		}))
		require.NoError(t, err)
		s, ok := got.([]any)
		require.True(t, ok)
		require.Len(t, s, 3)
		assert.InDelta(t, float64(1), s[0], 0.001)
		assert.Equal(t, "two", s[1])
		assert.Equal(t, true, s[2])
	})

	t.Run("empty tuple non-nil", func(t *testing.T) {
		got, err := ctyValueToGo(cty.EmptyTupleVal)
		require.NoError(t, err)
		s, ok := got.([]any)
		require.True(t, ok)
		assert.NotNil(t, s)
		assert.Empty(t, s)
	})

	t.Run("map is unsupported (loud-fail)", func(t *testing.T) {
		// cty.Map is unreachable from HCL literals (no tomap()/typed inputs).
		// If one surfaces, the default branch reports it as unsupported.
		_, err := ctyValueToGo(cty.MapVal(map[string]cty.Value{
			"a": cty.StringVal("x"),
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported variable type:")
	})

	t.Run("object returns Variables", func(t *testing.T) {
		got, err := ctyValueToGo(cty.ObjectVal(map[string]cty.Value{
			"a": cty.StringVal("x"),
		}))
		require.NoError(t, err)
		v, ok := got.(Variables)
		require.True(t, ok, "expected Variables, got %T", got)
		assert.Equal(t, "x", v["a"])
	})

	t.Run("null in collection produces path-aware error", func(t *testing.T) {
		_, err := ctyValueToGo(cty.TupleVal([]cty.Value{
			cty.StringVal("a"),
			cty.NullVal(cty.String),
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "[1]:")
		assert.Contains(t, err.Error(), "null values are not supported")
	})

	t.Run("null deep in nested object produces full path", func(t *testing.T) {
		// variables.inner[1].bad = null
		_, err := ctyValueToGo(cty.ObjectVal(map[string]cty.Value{
			"inner": cty.TupleVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{"good": cty.NumberIntVal(1)}),
				cty.ObjectVal(map[string]cty.Value{"bad": cty.NullVal(cty.String)}),
			}),
		}))
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, `"inner"`)
		assert.Contains(t, msg, "[1]:")
		assert.Contains(t, msg, `"bad"`)
		assert.Contains(t, msg, "null values are not supported")
	})

	t.Run("unsupported type at deep path is loud-fail with full path", func(t *testing.T) {
		// Capsule type is a stand-in for any non-handled cty type.
		caps := cty.Capsule("custom", reflect.TypeOf(struct{}{}))
		capsVal := cty.CapsuleVal(caps, &struct{}{})
		_, err := ctyValueToGo(cty.ObjectVal(map[string]cty.Value{
			"ports": cty.TupleVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{"weird": capsVal}),
			}),
		}))
		require.Error(t, err)
		msg := err.Error()
		// Path-aware wrapping
		assert.Contains(t, msg, `"ports"`)
		assert.Contains(t, msg, "[0]:")
		assert.Contains(t, msg, `"weird"`)
		// Magic string is part of the API contract — pin it.
		assert.Contains(t, msg, "unsupported variable type:")
	})

	t.Run("deeply nested mixed structure round-trips", func(t *testing.T) {
		// object → list → object → object → string (no Map; that branch is unsupported)
		v := cty.ObjectVal(map[string]cty.Value{
			"a": cty.TupleVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"b": cty.ObjectVal(map[string]cty.Value{
						"c": cty.StringVal("deep"),
					}),
				}),
			}),
		})
		got, err := ctyValueToGo(v)
		require.NoError(t, err)

		obj, ok := got.(Variables)
		require.True(t, ok)
		list, ok := obj["a"].([]any)
		require.True(t, ok)
		require.Len(t, list, 1)
		inner, ok := list[0].(Variables)
		require.True(t, ok)
		m, ok := inner["b"].(Variables)
		require.True(t, ok)
		assert.Equal(t, "deep", m["c"])
	})
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
				nested, ok := vars["nested"].(Variables)
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
		{
			name:   "list of objects (defaults)",
			wantOK: true,
			hcl: `variables = {
  ports = [
    { name = "tcp-foo", port = 8080 },
    { name = "tcp-bar", port = 9090 },
  ]
}`,
			check: func(t *testing.T, vars Variables) {
				ports, ok := vars["ports"].([]any)
				require.True(t, ok)
				require.Len(t, ports, 2)
				first, ok := ports[0].(Variables)
				require.True(t, ok)
				assert.Equal(t, "tcp-foo", first["name"])
			},
		},
		{
			name:   "primitive list (defaults)",
			wantOK: true,
			hcl:    `variables = { tags = ["a", "b"] }`,
			check: func(t *testing.T, vars Variables) {
				tags, ok := vars["tags"].([]any)
				require.True(t, ok)
				assert.Equal(t, []any{"a", "b"}, tags)
			},
		},
		{
			name:    "null in list rejected with path (defaults)",
			hcl:     `variables = { x = ["a", null] }`,
			wantErr: "[1]:",
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

			vars, err := ParseDefaults(t.Context(), path)

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
				// Nested object preserved as Variables
				logging, ok := cfg.Variables["logging"].(Variables)
				require.True(t, ok, "logging should decode to Variables")
				assert.True(t, logging["vectorEnabled"].(bool))
				assert.Equal(t, "collector", logging["vectorRole"])
				monitoring, ok := cfg.Variables["monitoring"].(Variables)
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
				a, ok := cfg.Variables["a"].(Variables)
				require.True(t, ok)
				b, ok := a["b"].(Variables)
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

		// ---- list of strings (tuple) ----
		{
			name:   "list of strings",
			wantOK: true,
			hcl:    `variables = { tags = ["a", "b", "c"] }`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				tags, ok := cfg.Variables["tags"].([]any)
				require.True(t, ok, "expected []any, got %T", cfg.Variables["tags"])
				assert.Equal(t, []any{"a", "b", "c"}, tags)
			},
		},

		// ---- list of numbers ----
		{
			name:   "list of numbers",
			wantOK: true,
			hcl:    `variables = { ports = [80, 443, 8080] }`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				ports, ok := cfg.Variables["ports"].([]any)
				require.True(t, ok)
				assert.Equal(t, []any{float64(80), float64(443), float64(8080)}, ports)
			},
		},

		// ---- empty list non-nil ----
		{
			name:   "empty list",
			wantOK: true,
			hcl:    `variables = { x = [] }`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				x, ok := cfg.Variables["x"].([]any)
				require.True(t, ok)
				assert.NotNil(t, x)
				assert.Empty(t, x)
			},
		},

		// ---- heterogeneous tuple ----
		{
			name:   "heterogeneous tuple",
			wantOK: true,
			hcl:    `variables = { x = [1, "two", true] }`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				x, ok := cfg.Variables["x"].([]any)
				require.True(t, ok)
				require.Len(t, x, 3)
				assert.InDelta(t, float64(1), x[0], 0.001)
				assert.Equal(t, "two", x[1])
				assert.Equal(t, true, x[2])
			},
		},

		// ---- list of homogeneous objects (TENANT_SERVICE_PORTS shape) ----
		{
			name:   "list of homogeneous objects",
			wantOK: true,
			hcl: `
variables = {
  ports = [
    { name = "tcp-foo", port = 8080 },
    { name = "tcp-bar", port = 9090 },
  ]
}`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				ports, ok := cfg.Variables["ports"].([]any)
				require.True(t, ok)
				require.Len(t, ports, 2)
				first, ok := ports[0].(Variables)
				require.True(t, ok, "list element must decode to Variables")
				assert.Equal(t, "tcp-foo", first["name"])
				assert.InDelta(t, float64(8080), first["port"], 0.001)
			},
		},

		// ---- list of heterogeneous objects (KEYCLOAK_EXTRA_VOLUMES shape) ----
		{
			name:   "list of heterogeneous objects",
			wantOK: true,
			hcl: `
variables = {
  vols = [
    { name = "tls-certs", secret = { secretName = "kc-tls" } },
    { name = "config" },
  ]
}`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				vols, ok := cfg.Variables["vols"].([]any)
				require.True(t, ok)
				require.Len(t, vols, 2)
				first, ok := vols[0].(Variables)
				require.True(t, ok)
				secret, ok := first["secret"].(Variables)
				require.True(t, ok)
				assert.Equal(t, "kc-tls", secret["secretName"])
				second, ok := vols[1].(Variables)
				require.True(t, ok)
				assert.Equal(t, "config", second["name"])
			},
		},

		// ---- nested object containing list ----
		{
			name:   "nested object containing list",
			wantOK: true,
			hcl: `
variables = {
  keycloak = {
    extraVolumes = [
      { name = "x" },
    ]
  }
}`,
			check: func(t *testing.T, cfg *UDSBundleConfig) {
				keycloak, ok := cfg.Variables["keycloak"].(Variables)
				require.True(t, ok)
				vols, ok := keycloak["extraVolumes"].([]any)
				require.True(t, ok)
				require.Len(t, vols, 1)
			},
		},

		// ---- null in collection produces path-aware error ----
		{
			name:    "null in list rejected with index",
			hcl:     `variables = { x = ["a", null] }`,
			wantErr: "[1]:",
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

			cfg, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleConfig(t.Context(), path)

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

func TestParseBundleConfig_FileFunction(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.uds.hcl")
	relativePath := filepath.Join(dir, "config-value.txt")
	require.NoError(t, os.WriteFile(relativePath, []byte("from file"), tmpFilePerm))

	require.NoError(t, os.WriteFile(configPath, []byte(`
options {
  tmp_dir = file("config-value.txt")
}

variables = {
  direct = file("config-value.txt")
  nested = {
    rendered = "value: ${file("config-value.txt")}"
  }
}
`), tmpFilePerm))

	cfg, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleConfig(t.Context(), configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.Options)
	assert.Equal(t, "from file", cfg.Options.TmpDir)
	assert.Equal(t, "from file", cfg.Variables["direct"])
	nested, ok := cfg.Variables["nested"].(Variables)
	require.True(t, ok)
	assert.Equal(t, "value: from file", nested["rendered"])
}

func TestParseBundleConfig_FileFunctionErrors(t *testing.T) {
	dir := t.TempDir()
	invalidUTF8Path := filepath.Join(dir, "invalid.txt")
	require.NoError(t, os.WriteFile(invalidUTF8Path, []byte{0xff}, tmpFilePerm))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "a-directory"), tempDirPerm))

	tests := []struct {
		name    string
		expr    string
		wantErr string
	}{
		{
			name:    "non-string argument",
			expr:    "file([])",
			wantErr: "Invalid function argument",
		},
		{
			name:    "missing file",
			expr:    `file("missing.txt")`,
			wantErr: "stat file",
		},
		{
			name:    "directory",
			expr:    `file("a-directory")`,
			wantErr: "not a regular file",
		},
		{
			name:    "invalid UTF-8",
			expr:    `file("invalid.txt")`,
			wantErr: "not valid UTF-8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(dir, "config.uds.hcl")
			require.NoError(t, os.WriteFile(configPath, []byte("variables = { value = "+tt.expr+" }"), tmpFilePerm))

			_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleConfig(t.Context(), configPath)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseBundleAndDefaultsSupportFileFunction(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
		parse   func(context.Context, string) error
	}{
		{
			name: "bundle direct expression",
			file: BundleFileName,
			content: `uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = file("name.txt") }
package "example" { source = "oci://example.com/package:v1" }`,
			parse: func(ctx context.Context, path string) error {
				_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(ctx, path)
				return err
			},
		},
		{
			name: "bundle local expression",
			file: BundleFileName,
			content: `locals { name = file("name.txt") }
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = local.name }
package "example" { source = "oci://example.com/package:v1" }`,
			parse: func(ctx context.Context, path string) error {
				_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleFile(ctx, path)
				return err
			},
		},
		{
			name:    "defaults nested template expression",
			file:    BundleDefaultsFileName,
			content: `variables = { nested = { value = "${file("value.txt")}" } }`,
			parse: func(ctx context.Context, path string) error {
				_, err := ParseDefaults(ctx, path)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tt.file)
			requiredFile := "name.txt"
			if tt.file == BundleDefaultsFileName {
				requiredFile = "value.txt"
			}
			require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(path), requiredFile), []byte("example"), tmpFilePerm))
			require.NoError(t, os.WriteFile(path, []byte(tt.content), tmpFilePerm))

			err := tt.parse(t.Context(), path)
			require.NoError(t, err)
		})
	}
}

func TestMaterializeBundleFileFunctions(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "name.txt"), []byte("from\nfile\""), tmpFilePerm))
	path := filepath.Join(dir, BundleFileName)
	require.NoError(t, os.WriteFile(path, []byte(`
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = file("name.txt") }
package "example" { source = "oci://example.com/package:v1" }
`), tmpFilePerm))

	p := NewHCLParser("", iostreams.IOStreams{})
	_, materialized, err := p.parseAndMaterializeBundleFile(t.Context(), path)
	require.NoError(t, err)
	assert.NotContains(t, string(materialized), "file(")
	_, err = p.ParseBundleBytes(t.Context(), materialized)
	require.NoError(t, err)
}

func TestMaterializeBundleFileFunctionsPreservesSourceAfterReplacement(t *testing.T) {
	dir := t.TempDir()
	description := strings.Repeat("materialized description ", 4)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "description.txt"), []byte(description), tmpFilePerm))
	path := filepath.Join(dir, BundleFileName)
	require.NoError(t, os.WriteFile(path, []byte(`
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata {
  name = "example"
  description = file("description.txt")
  version = "1.2.3"
}
package "example" { source = "oci://example.com/package:v1" }
`), tmpFilePerm))

	p := NewHCLParser("", iostreams.IOStreams{})
	_, materialized, err := p.parseAndMaterializeBundleFile(t.Context(), path)
	require.NoError(t, err)
	parsed, err := p.ParseBundleBytes(t.Context(), materialized)
	require.NoError(t, err)
	assert.Equal(t, description, parsed.Metadata.Description)
	assert.Equal(t, "1.2.3", parsed.Metadata.Version)
	require.Len(t, parsed.Packages, 1)
	assert.Equal(t, "oci://example.com/package:v1", parsed.Packages[0].Source)
}

func TestMaterializeBundleFileFunctionsUsesSourceOrderLocalValues(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("from a"), tmpFilePerm))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("from b"), tmpFilePerm))
	path := filepath.Join(dir, BundleFileName)
	require.NoError(t, os.WriteFile(path, []byte(`
uds { bundle_api_version = "uds.dev/v1alpha1" }
locals {
  path = "a.txt"
  name = file(local.path)
}
locals {
  path = "b.txt"
}
metadata { name = local.name }
package "example" { source = "oci://example.com/package:v1" }
`), tmpFilePerm))

	p := NewHCLParser("", iostreams.IOStreams{})
	bundle, materialized, err := p.parseAndMaterializeBundleFile(t.Context(), path)
	require.NoError(t, err)
	assert.Equal(t, "from a", bundle.Metadata.Name)
	assert.NotContains(t, string(materialized), "file(")
	parsed, err := p.ParseBundleBytes(t.Context(), materialized)
	require.NoError(t, err)
	assert.Equal(t, bundle.Metadata.Name, parsed.Metadata.Name)
}

func TestMaterializeDefaultsFilePreservesExpressionScopeAndLaziness(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, BundleDefaultsFileName)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("from file"), tmpFilePerm))
	require.NoError(t, os.WriteFile(path, []byte(`
variables = {
  values = [for name in ["a"] : file("${name}.txt")]
  value  = false ? file("missing.txt") : "ok"
}
`), tmpFilePerm))

	materialized, err := materializeDefaultsFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(materialized), "file(")

	materializedPath := filepath.Join(dir, "materialized.uds.hcl")
	require.NoError(t, os.WriteFile(materializedPath, materialized, tmpFilePerm))
	variables, err := ParseDefaults(t.Context(), materializedPath)
	require.NoError(t, err)
	assert.Equal(t, []any{"from file"}, variables["values"])
	assert.Equal(t, "ok", variables["value"])
}

func TestParseBundleBytesRejectsFileFunction(t *testing.T) {
	_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleBytes(t.Context(), []byte(`
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = file("name.txt") }
package "example" { source = "oci://example.com/package:v1" }
`))
	require.ErrorContains(t, err, "requires a file-backed bundle source")
}
