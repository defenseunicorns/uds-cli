// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"os"
	"path/filepath"
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

	// core_base: locals fully resolved
	assert.Equal(t, "core_base", b.Packages[0].Name)
	wantSource := "oci://ghcr.io/defenseunicorns/packages/uds/core-base:0.59.1-upstream"
	assert.Equal(t, wantSource, b.Packages[0].Source)
	assert.Equal(t, []string{"istio-passthrough-gateway", "istio-egress-gateway"}, b.Packages[0].OptionalComponents)

	// core_logging: depends_on and valuesFiles
	assert.Equal(t, "core_logging", b.Packages[1].Name)
	require.Len(t, b.Packages[1].DependsOn, 1)
	assert.Equal(t, "core_base", b.Packages[1].DependsOn[0].Name)
	assert.Equal(t, []string{"values/loki.yaml", "values/vector.yaml"}, b.Packages[1].ValueFiles)

	// core_monitoring: namespace, depends_on with 2 entries
	assert.Equal(t, "monitoring", b.Packages[2].Namespace)
	require.Len(t, b.Packages[2].DependsOn, 2)
	depNames := []string{b.Packages[2].DependsOn[0].Name, b.Packages[2].DependsOn[1].Name}
	assert.Contains(t, depNames, "core_base")
	assert.Contains(t, depNames, "core_logging")
	assert.Equal(t, []string{"values/monitoring.yaml"}, b.Packages[2].ValueFiles)
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
	assert.Empty(t, b.Packages[0].ValueFiles)
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
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp HCL file: %v", err)
	}
	return path
}
