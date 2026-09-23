// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicBundleValidationContracts(t *testing.T) {
	t.Parallel()

	valid := spec.UDSBundle{
		UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: spec.Metadata{Name: "example", Version: "1.0.0"},
		Packages: []spec.Package{
			{Name: "base", Source: "oci://example.com/base:v1"},
			{Name: "app", Source: "oci://example.com/app:v1", DependsOn: []spec.PackageRef{{Name: "base"}}, OptionalComponents: []string{"extras"}},
		},
	}
	require.NoError(t, valid.Validate())

	missing := (&spec.UDSBundle{}).Validate()
	require.Error(t, missing)
	require.ErrorIs(t, missing, spec.ErrBundleAPIVersionRequired)
	require.ErrorIs(t, missing, spec.ErrMetadataNameRequired)
	require.ErrorIs(t, missing, spec.ErrPackagesRequired)

	invalid := spec.UDSBundle{
		UDS: spec.UDSBlock{BundleAPIVersion: "uds.dev/v0"},
		Packages: []spec.Package{
			{Name: "", Source: "oci://example.com/unnamed:v1"},
			{Name: "bad/name", Source: "oci://example.com/bad:v1", OptionalComponents: []string{"", "dup", "dup"}},
			{Name: "bad/name", Source: "oci://example.com/duplicate:v1"},
			{Name: "self", Source: "oci://example.com/self:v1", DependsOn: []spec.PackageRef{{Name: "self"}}},
			{Name: "known", Source: "oci://example.com/known:v1", DependsOn: []spec.PackageRef{{Name: "missing"}}},
			{Name: "missing-source"},
		},
	}
	joined := invalid.Validate()
	require.Error(t, joined)
	assert.Contains(t, joined.Error(), `uds.bundle_api_version "uds.dev/v0"`)
	assert.Contains(t, joined.Error(), `package "missing-source": source is required`)

	var unsupported *spec.UnsupportedBundleAPIVersionError
	require.ErrorAs(t, joined, &unsupported)
	assert.Equal(t, "uds.dev/v0", unsupported.Actual)
	assert.Equal(t, "uds.dev/v1alpha1", unsupported.Expected)

	var packageName *spec.PackageNameRequiredError
	require.ErrorAs(t, joined, &packageName)
	assert.Equal(t, 0, packageName.Index)

	var invalidName *spec.InvalidPackageNameError
	require.ErrorAs(t, joined, &invalidName)
	assert.Equal(t, 1, invalidName.Index)
	assert.Equal(t, "bad/name", invalidName.Name)

	var duplicateName *spec.DuplicatePackageNameError
	require.ErrorAs(t, joined, &duplicateName)
	assert.Equal(t, 2, duplicateName.Index)
	assert.Equal(t, "bad/name", duplicateName.Name)

	var missingSource *spec.PackageSourceRequiredError
	require.ErrorAs(t, joined, &missingSource)
	assert.Equal(t, "missing-source", missingSource.Package)

	var selfDependency *spec.SelfDependencyError
	require.ErrorAs(t, joined, &selfDependency)
	assert.Equal(t, "self", selfDependency.Package)

	var unknownDependency *spec.UnknownDependencyError
	require.ErrorAs(t, joined, &unknownDependency)
	assert.Equal(t, "known", unknownDependency.Package)
	assert.Equal(t, "missing", unknownDependency.Dependency)

	var emptyComponent *spec.EmptyOptionalComponentError
	require.ErrorAs(t, joined, &emptyComponent)
	assert.Equal(t, "bad/name", emptyComponent.Package)

	var duplicateComponent *spec.DuplicateOptionalComponentError
	require.ErrorAs(t, joined, &duplicateComponent)
	assert.Equal(t, "bad/name", duplicateComponent.Package)
	assert.Equal(t, "dup", duplicateComponent.Component)

	assert.ErrorIs(t, joined, spec.ErrMetadataNameRequired)
}
