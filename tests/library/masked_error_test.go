// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/stretchr/testify/require"
)

func TestMaskedDeployErrorKeepsTypedDetails(t *testing.T) {
	const secret = "middle"
	cfg := newTestConfig()
	cfg.Variables = bundle.Variables{"credential": []any{secret, "package", "bundle"}}
	cfg.SensitiveVariables = map[string]bool{"credential": true}
	source := &bundle.DeploySource{Bundle: threePkgChainBundle()}
	opts := bundle.DeployOptions{Config: cfg, Packages: []string{"leaf"}}

	_, err := bundle.Deploy(t.Context(), source, opts)
	var masked *bundle.MaskedError
	require.ErrorAs(t, err, &masked)
	require.NotContains(t, err.Error(), secret)
	var violation *bundle.DependencyViolationError
	require.ErrorAs(t, err, &violation)
	require.Equal(t, []string{secret}, violation.Violations["leaf"])
	require.NotContains(t, violation.Error(), secret)
	require.NotContains(t, violation.Error(), "package")

	source.Bundle.UDS.BundleAPIVersion = secret
	_, err = bundle.Deploy(t.Context(), source, opts)
	require.ErrorAs(t, err, &masked)
	require.NotContains(t, err.Error(), secret)
	var version *spec.UnsupportedBundleAPIVersionError
	require.ErrorAs(t, err, &version)
	require.Equal(t, secret, version.Actual)
	require.Equal(t, "uds.dev/v1alpha1", version.Expected)
	require.NotContains(t, version.Error(), secret)
	require.NotContains(t, version.Error(), "bundle")
}

func TestCreateReturnsPublicHCLDiagnostic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(path, []byte("metadata { name = sensitive( }"), 0o600))
	_, err := bundle.Create(t.Context(), path, bundle.CreateOptions{
		Config: newTestConfig(), Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	var diag *bundle.HCLDiagnosticError
	require.ErrorAs(t, err, &diag)
	require.Equal(t, 1, diag.Line)
	require.Positive(t, diag.Column)
}
