// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestUDSBundle_ToInspectResult(t *testing.T) {
	b := &UDSBundle{
		Metadata: Metadata{
			Name:        "test-bundle",
			Description: "A test bundle",
			Version:     "2.0.0",
		},
		Packages: []Package{
			{
				Name:      "db",
				Source:    "oci://ghcr.io/org/db:1.0",
				Namespace: "database",
				DependsOn: nil,
			},
			{
				Name:        "api",
				Source:      "oci://ghcr.io/org/api:2.0",
				DependsOn:   []PackageRef{pkgRef("db")},
				ValuesFiles: []string{"values/api.yaml"},
			},
		},
	}

	result, err := b.ToInspectResult()
	require.NoError(t, err)

	assert.Equal(t, "test-bundle", result.Name)
	assert.Equal(t, "A test bundle", result.Description)
	assert.Equal(t, "2.0.0", result.Version)
	require.Len(t, result.Packages, 2)

	assert.Equal(t, "db", result.Packages[0].Name)
	assert.Equal(t, "oci://ghcr.io/org/db:1.0", result.Packages[0].Source)
	assert.Equal(t, "database", result.Packages[0].Namespace)
	assert.Nil(t, result.Packages[0].DependsOn)

	assert.Equal(t, "api", result.Packages[1].Name)
	assert.Equal(t, "oci://ghcr.io/org/api:2.0", result.Packages[1].Source)
	assert.Empty(t, result.Packages[1].Namespace)
	assert.Equal(t, []string{"db"}, result.Packages[1].DependsOn)
	assert.Equal(t, []string{"values/api.yaml"}, result.Packages[1].ValuesFiles)
}

// TestUDSBundle_ToInspectResult_DAGOrder verifies that ToInspectResult returns
// packages in DAG (topological) order, not declaration order. Packages declared
// "out of order" in the HCL file must appear sorted by dependency level, and
// packages within the same level must be sorted alphabetically for determinism.
func TestUDSBundle_ToInspectResult_DAGOrder(t *testing.T) {
	// Declaration order: D, C, B, A — completely reversed from deployment order.
	// Dependency chain: A (no deps) → B depends on A → C depends on B → D depends on C
	b := &UDSBundle{
		Metadata: Metadata{Name: "dag-test"},
		Packages: []Package{
			{Name: "D", Source: "oci://example.com/D:v1", DependsOn: []PackageRef{pkgRef("C")}},
			{Name: "C", Source: "oci://example.com/C:v1", DependsOn: []PackageRef{pkgRef("B")}},
			{Name: "B", Source: "oci://example.com/B:v1", DependsOn: []PackageRef{pkgRef("A")}},
			{Name: "A", Source: "oci://example.com/A:v1"},
		},
	}

	result, err := b.ToInspectResult()
	require.NoError(t, err)

	require.Len(t, result.Packages, 4)
	assert.Equal(t, "A", result.Packages[0].Name, "level 0: A has no dependencies")
	assert.Equal(t, "B", result.Packages[1].Name, "level 1: B depends on A")
	assert.Equal(t, "C", result.Packages[2].Name, "level 2: C depends on B")
	assert.Equal(t, "D", result.Packages[3].Name, "level 3: D depends on C")
}

// TestUDSBundle_ToInspectResult_DAGOrder_Deterministic verifies that packages
// within the same DAG level are sorted alphabetically for deterministic output.
func TestUDSBundle_ToInspectResult_DAGOrder_Deterministic(t *testing.T) {
	// Diamond: A (no deps), B and C both depend on A, D depends on B and C.
	// Declaration order deliberately puts C before B to test alphabetical sorting.
	b := &UDSBundle{
		Metadata: Metadata{Name: "diamond"},
		Packages: []Package{
			{Name: "D", Source: "oci://example.com/D:v1", DependsOn: []PackageRef{pkgRef("B"), pkgRef("C")}},
			{Name: "C", Source: "oci://example.com/C:v1", DependsOn: []PackageRef{pkgRef("A")}},
			{Name: "A", Source: "oci://example.com/A:v1"},
			{Name: "B", Source: "oci://example.com/B:v1", DependsOn: []PackageRef{pkgRef("A")}},
		},
	}

	result, err := b.ToInspectResult()
	require.NoError(t, err)

	require.Len(t, result.Packages, 4)
	// Level 0
	assert.Equal(t, "A", result.Packages[0].Name, "level 0: A has no dependencies")
	// Level 1: B and C in alphabetical order
	assert.Equal(t, "B", result.Packages[1].Name, "level 1: B before C alphabetically")
	assert.Equal(t, "C", result.Packages[2].Name, "level 1: C after B alphabetically")
	// Level 2
	assert.Equal(t, "D", result.Packages[3].Name, "level 2: D depends on B and C")
}

func TestUDSBundle_ToInspectResult_Empty(t *testing.T) {
	b := &UDSBundle{
		Metadata: Metadata{Name: "empty"},
	}

	result, err := b.ToInspectResult()
	require.NoError(t, err)
	assert.Equal(t, "empty", result.Name)
	assert.Empty(t, result.Packages)
}

func TestPackageSummary_NoDependsOnOmittedInJSON(t *testing.T) {
	pkg := PackageSummary{Name: "solo", Source: "oci://example.com/solo:v1"}
	data, err := json.Marshal(pkg)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "dependsOn")
}

func TestInspectResult_JSONRoundTrip(t *testing.T) {
	original := &InspectResult{
		Name:    "test",
		Version: "1.0",
		Packages: []PackageSummary{
			{Name: "pkg1", Source: "oci://example.com/pkg:v1", DependsOn: []string{"pkg0"}},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded InspectResult
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Version, decoded.Version)
	require.Len(t, decoded.Packages, 1)
	assert.Equal(t, "pkg1", decoded.Packages[0].Name)
}

func TestInspectResult_YAMLRoundTrip(t *testing.T) {
	original := &InspectResult{
		Name:    "test",
		Version: "1.0",
		Packages: []PackageSummary{
			{Name: "pkg1", Source: "oci://example.com/pkg:v1"},
		},
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var decoded InspectResult
	require.NoError(t, yaml.Unmarshal(data, &decoded))
	assert.Equal(t, original.Name, decoded.Name)
}

