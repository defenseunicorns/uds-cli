// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackageSignatureVerificationConfigHelpers(t *testing.T) {
	tests := []struct {
		name                           string
		pkg                            Package
		hasPublicKey                   bool
		hasKeylessConfig               bool
		hasCertificateIdentityConfig   bool
		hasCertificateOIDCIssuerConfig bool
	}{
		{
			name:                           "empty package",
			pkg:                            Package{},
			hasPublicKey:                   false,
			hasKeylessConfig:               false,
			hasCertificateIdentityConfig:   false,
			hasCertificateOIDCIssuerConfig: false,
		},
		{
			name:                           "public key only",
			pkg:                            Package{PublicKey: "fake-key"},
			hasPublicKey:                   true,
			hasKeylessConfig:               false,
			hasCertificateIdentityConfig:   false,
			hasCertificateOIDCIssuerConfig: false,
		},
		{
			name:                           "empty keyless verification config only",
			pkg:                            Package{KeylessVerification: &KeylessVerification{}},
			hasPublicKey:                   false,
			hasKeylessConfig:               true,
			hasCertificateIdentityConfig:   false,
			hasCertificateOIDCIssuerConfig: false,
		},
		{
			name:                           "certificate identity only",
			pkg:                            Package{KeylessVerification: &KeylessVerification{CertificateIdentity: "https://example.com"}},
			hasPublicKey:                   false,
			hasKeylessConfig:               true,
			hasCertificateIdentityConfig:   true,
			hasCertificateOIDCIssuerConfig: false,
		},
		{
			name:                           "certificate identity regexp only",
			pkg:                            Package{KeylessVerification: &KeylessVerification{CertificateIdentityRegexp: "https://.*"}},
			hasPublicKey:                   false,
			hasKeylessConfig:               true,
			hasCertificateIdentityConfig:   true,
			hasCertificateOIDCIssuerConfig: false,
		},
		{
			name:                           "certificate OIDC issuer only",
			pkg:                            Package{KeylessVerification: &KeylessVerification{CertificateOIDCIssuer: "https://token.actions.githubusercontent.com"}},
			hasPublicKey:                   false,
			hasKeylessConfig:               true,
			hasCertificateIdentityConfig:   false,
			hasCertificateOIDCIssuerConfig: true,
		},
		{
			name:                           "certificate OIDC issuer regexp only",
			pkg:                            Package{KeylessVerification: &KeylessVerification{CertificateOIDCIssuerRegexp: "https://.*"}},
			hasPublicKey:                   false,
			hasKeylessConfig:               true,
			hasCertificateIdentityConfig:   false,
			hasCertificateOIDCIssuerConfig: true,
		},
		{
			name:                           "public key and keyless config",
			pkg:                            Package{PublicKey: "fake-key", KeylessVerification: &KeylessVerification{}},
			hasPublicKey:                   true,
			hasKeylessConfig:               true,
			hasCertificateIdentityConfig:   false,
			hasCertificateOIDCIssuerConfig: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.hasPublicKey, tt.pkg.HasPublicKey())
			require.Equal(t, tt.hasKeylessConfig, tt.pkg.HasKeylessConfig())
			require.Equal(t, tt.hasCertificateIdentityConfig, tt.pkg.HasCertificateIdentityConfig())
			require.Equal(t, tt.hasCertificateOIDCIssuerConfig, tt.pkg.HasCertificateOIDCIssuerConfig())
		})
	}
}
