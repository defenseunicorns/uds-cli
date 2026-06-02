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

func TestPackageHasKeylessConfig(t *testing.T) {
	require.False(t, Package{}.HasKeylessConfig())
	require.True(t, Package{KeylessVerification: &KeylessVerification{}}.HasKeylessConfig())
}

func TestPackageHasCertificateIdentityConfig(t *testing.T) {
	require.False(t, Package{}.HasCertificateIdentityConfig())
	require.True(t, Package{KeylessVerification: &KeylessVerification{CertificateIdentity: "https://example.com"}}.HasCertificateIdentityConfig())
	require.True(t, Package{KeylessVerification: &KeylessVerification{CertificateIdentityRegexp: "https://.*"}}.HasCertificateIdentityConfig())
}

func TestPackageHasCertificateOIDCIssuerConfig(t *testing.T) {
	require.False(t, Package{}.HasCertificateOIDCIssuerConfig())
	require.True(t, Package{KeylessVerification: &KeylessVerification{CertificateOIDCIssuer: "https://token.actions.githubusercontent.com"}}.HasCertificateOIDCIssuerConfig())
	require.True(t, Package{KeylessVerification: &KeylessVerification{CertificateOIDCIssuerRegexp: "https://.*"}}.HasCertificateOIDCIssuerConfig())
}
