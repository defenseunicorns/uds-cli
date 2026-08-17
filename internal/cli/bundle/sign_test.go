// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyOptionsPolicy_TrustedRootOverridePreservesConfiguredKeylessPolicy(t *testing.T) {
	trustedRootPath := filepath.Join(t.TempDir(), "trusted-root.json")
	require.NoError(t, os.WriteFile(trustedRootPath, []byte("custom root"), 0o600))

	configuredKeyless := &bundlepkg.KeylessVerification{
		CertificateIdentity:   "https://github.com/defenseunicorns",
		CertificateOIDCIssuer: "https://token.actions.githubusercontent.com",
		TrustedRoot:           "configured root",
	}
	o := VerifyOptions{
		Config: &bundlepkg.UDSBundleConfig{
			SignatureVerification: &bundlepkg.VerificationPolicy{Keyless: configuredKeyless},
		},
		TrustedRoot: trustedRootPath,
	}

	policy, err := o.policy()

	require.NoError(t, err)
	require.NotNil(t, policy.Keyless)
	assert.Equal(t, configuredKeyless.CertificateIdentity, policy.Keyless.CertificateIdentity)
	assert.Equal(t, configuredKeyless.CertificateOIDCIssuer, policy.Keyless.CertificateOIDCIssuer)
	assert.Equal(t, "custom root", policy.Keyless.TrustedRoot)
	assert.Equal(t, "configured root", configuredKeyless.TrustedRoot)
}
