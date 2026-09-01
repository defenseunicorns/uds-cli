// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArtifactPackageNames(t *testing.T) {
	b := &spec.UDSBundle{Packages: []spec.Package{
		{Name: "primary", Source: "./original-primary.tar.zst"},
		{Name: "secondary", Source: "oci://original.example/secondary:1"},
	}}
	source := &DeploySource{packageZarfNames: map[string]string{
		"primary":   "deployed-primary",
		"secondary": "deployed-secondary",
	}}
	names, err := artifactPackageNames(source, b, []string{"primary"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"primary": "deployed-primary"}, names)
}

func TestArtifactPackageNamesRejectsMissingEmbeddedName(t *testing.T) {
	b := &spec.UDSBundle{Packages: []spec.Package{{Name: "primary", Source: "./original-primary.tar.zst"}}}
	source := &DeploySource{packageZarfNames: map[string]string{}}
	_, err := artifactPackageNames(source, b, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embedded Zarf package name is required")
}

func TestArtifactPackageNamesSourceBundleDoesNotOverrideSources(t *testing.T) {
	b := &spec.UDSBundle{Packages: []spec.Package{{Name: "primary", Source: "./original-primary.tar.zst"}}}
	names, err := artifactPackageNames(&DeploySource{}, b, nil)
	require.NoError(t, err)
	assert.Nil(t, names)
}
