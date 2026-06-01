// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackageHasPublicKey(t *testing.T) {
	require.False(t, Package{}.HasPublicKey())
	require.True(t, Package{PublicKey: "fake-key"}.HasPublicKey())
}

func TestPackageHasCertificateIdentityConfig(t *testing.T) {
	require.False(t, Package{}.HasCertificateIdentityConfig())
	require.True(t, Package{CertificateIdentity: "https://example.com"}.HasCertificateIdentityConfig())
	require.True(t, Package{CertificateIdentityRegexp: "https://.*"}.HasCertificateIdentityConfig())
}

func TestPackageHasCertificateOIDCIssuerConfig(t *testing.T) {
	require.False(t, Package{}.HasCertificateOIDCIssuerConfig())
	require.True(t, Package{CertificateOIDCIssuer: "https://token.actions.githubusercontent.com"}.HasCertificateOIDCIssuerConfig())
	require.True(t, Package{CertificateOIDCIssuerRegexp: "https://.*"}.HasCertificateOIDCIssuerConfig())
}

func TestPackageHasKeylessModifierConfig(t *testing.T) {
	require.False(t, Package{}.HasKeylessModifierConfig())
	require.True(t, Package{TrustedRoot: `{"mediaType":"application/vnd.dev.sigstore.trustedroot+json"}`}.HasKeylessModifierConfig())
	require.True(t, Package{SkipTLogVerify: true}.HasKeylessModifierConfig())
	require.True(t, Package{UseSignedTimestamps: true}.HasKeylessModifierConfig())
	require.False(t, Package{CertificateIdentity: "https://example.com"}.HasKeylessModifierConfig())
}

func TestPackageHasKeylessConfig(t *testing.T) {
	require.False(t, Package{}.HasKeylessConfig())
	require.True(t, Package{CertificateIdentity: "https://example.com"}.HasKeylessConfig())
	require.True(t, Package{CertificateIdentityRegexp: "https://.*"}.HasKeylessConfig())
	require.True(t, Package{CertificateOIDCIssuer: "https://token.actions.githubusercontent.com"}.HasKeylessConfig())
	require.True(t, Package{CertificateOIDCIssuerRegexp: "https://.*"}.HasKeylessConfig())
	require.True(t, Package{TrustedRoot: `{"mediaType":"application/vnd.dev.sigstore.trustedroot+json"}`}.HasKeylessConfig())
	require.True(t, Package{SkipTLogVerify: true}.HasKeylessConfig())
	require.True(t, Package{UseSignedTimestamps: true}.HasKeylessConfig())
}
