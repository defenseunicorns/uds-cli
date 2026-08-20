// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validValidationConfig returns baseline valid public configuration for tests.
func validValidationConfig() *UDSBundleConfig {
	return &UDSBundleConfig{
		Options: &ConfigOptions{LogLevel: "info", Concurrency: 10},
	}
}

func TestValidateOperationOptions(t *testing.T) {
	tests := []struct {
		name     string
		validate func() error
		wantErr  string
	}{
		{name: "deploy requires config", validate: func() error { return (DeployOptions{}).Validate() }, wantErr: "config is required"},
		{name: "deploy accepts config", validate: func() error { return (DeployOptions{Config: validValidationConfig()}).Validate() }},
		{name: "deploy package requires config", validate: func() error { return (DeployPackageOptions{}).Validate() }, wantErr: "config is required"},
		{name: "deploy package requires bundle directory", validate: func() error {
			return (DeployPackageOptions{Config: validValidationConfig()}).Validate()
		}, wantErr: "BundleDir is required"},
		{name: "deploy package accepts valid options", validate: func() error {
			return (DeployPackageOptions{Config: validValidationConfig(), BundleDir: "/bundle"}).Validate()
		}},
		{name: "remove requires config", validate: func() error { return (RemoveOptions{}).Validate() }, wantErr: "config is required"},
		{name: "remove accepts config", validate: func() error {
			return (RemoveOptions{Config: validValidationConfig()}).Validate()
		}},
		{name: "remove package requires config", validate: func() error { return (removePackageOptions{}).validate() }, wantErr: "config is required"},
		{name: "remove package accepts config", validate: func() error {
			return (removePackageOptions{Config: validValidationConfig()}).validate()
		}},
		{name: "create requires config", validate: func() error { return (CreateOptions{}).Validate() }, wantErr: "config is required"},
		{name: "create accepts config", validate: func() error {
			return (CreateOptions{Config: validValidationConfig(), Signing: SigningOptions{Mode: SigningModeUnsigned}}).Validate()
		}},
		{name: "pull requires config", validate: func() error { return (PullOptions{}).Validate() }, wantErr: "config is required"},
		{name: "pull accepts config", validate: func() error { return (PullOptions{Config: validValidationConfig()}).Validate() }},
		{name: "push requires config", validate: func() error { return (PushOptions{}).Validate() }, wantErr: "config is required"},
		{name: "push accepts config", validate: func() error { return (PushOptions{Config: validValidationConfig()}).Validate() }},
		{name: "reconfigure requires config", validate: func() error { return (ReconfigureOptions{Suffix: "-v2"}).Validate() }, wantErr: "config is required"},
		{name: "reconfigure rejects invalid suffix", validate: func() error {
			return (ReconfigureOptions{Config: validValidationConfig(), Suffix: "v2"}).Validate()
		}, wantErr: "invalid suffix"},
		{name: "reconfigure accepts valid options", validate: func() error {
			return (ReconfigureOptions{Config: validValidationConfig(), Suffix: "-v2", Signing: SigningOptions{Mode: SigningModeUnsigned}}).Validate()
		}},
		{name: "inspect rejects unsupported source", validate: func() error {
			return (InspectOptions{Source: "bundle.uds.hcl", Config: validValidationConfig()}).Validate()
		}, wantErr: "source must be a .tar.zst bundle artifact or OCI reference"},
		{name: "inspect rejects malformed OCI reference", validate: func() error {
			return (InspectOptions{Source: "oci://", Config: validValidationConfig()}).Validate()
		}, wantErr: "parsing OCI reference"},
		{name: "inspect accepts bundle artifact source", validate: func() error {
			return (InspectOptions{Source: "bundle.tar.zst", Config: validValidationConfig()}).Validate()
		}},
		{name: "inspect accepts OCI source", validate: func() error {
			return (InspectOptions{Source: "ghcr.io/example/bundle:v1", Config: validValidationConfig()}).Validate()
		}},
		{name: "inspect validates configured verification policy", validate: func() error {
			config := validValidationConfig()
			config.SignatureVerification = &VerificationPolicy{
				Keyless: &KeylessVerification{CertificateIdentity: "workflow"},
			}
			return (InspectOptions{Source: "bundle.tar.zst", Config: config}).Validate()
		}, wantErr: "certificate OIDC issuer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidationErrorContracts(t *testing.T) {
	t.Parallel()

	require.ErrorIs(t, (DeployOptions{}).Validate(), ErrInvalidConfig)
	require.ErrorIs(t, (DeployPackageOptions{Config: validValidationConfig()}).Validate(), ErrBundleDirRequired)
	require.ErrorIs(t, (ReconfigureOptions{Config: validValidationConfig()}).Validate(), ErrInvalidSuffix)
	require.ErrorIs(t, (SigningOptions{Mode: SigningModeKey}).Validate(), ErrInvalidSigningOptions)
	require.ErrorIs(t, (VerificationPolicy{}).Validate(), ErrInvalidVerificationPolicy)
}

func TestFormatDependencyError(t *testing.T) {
	violations := map[string][]string{
		"core":  {"nginx", "podinfo"},
		"nginx": {"podinfo"},
	}
	err := formatDependencyError("cannot remove package(s) with bundle dependents", "is required by", violations)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "cannot remove package(s) with bundle dependents:")
	assert.Contains(t, msg, `"core" is required by: nginx, podinfo`)
	assert.Contains(t, msg, `"nginx" is required by: podinfo`)
	assert.NotContains(t, msg, "--force")
	assert.Less(t, strings.Index(msg, `"core"`), strings.Index(msg, `"nginx"`))

	var violationError *DependencyViolationError
	require.ErrorAs(t, err, &violationError)
	assert.Equal(t, "cannot remove package(s) with bundle dependents", violationError.header)
	assert.Equal(t, "is required by", violationError.relation)
	assert.Equal(t, violations, violationError.Violations)
}
