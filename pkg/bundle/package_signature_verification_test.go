// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

func TestValidatePackageSignatureVerification(t *testing.T) {
	falseValue := false
	trueValue := true
	validKeyless := &KeylessSignatureVerification{
		CertificateIdentity:   "https://example.com/workflow",
		CertificateOIDCIssuer: "https://issuer.example.com",
	}
	tests := []struct {
		name         string
		verification *PackageSignatureVerification
		wantErr      string
	}{
		{name: "missing block", wantErr: "signature_verification block is required"},
		{name: "verify false", verification: &PackageSignatureVerification{Verify: &falseValue}},
		{name: "public key", verification: &PackageSignatureVerification{PublicKey: "public key"}},
		{name: "blank public key", verification: &PackageSignatureVerification{PublicKey: " \t\n", Keyless: validKeyless}, wantErr: "public_key must not be blank"},
		{name: "keyless", verification: &PackageSignatureVerification{Keyless: validKeyless}},
		{name: "verify true public key", verification: &PackageSignatureVerification{Verify: &trueValue, PublicKey: "public key"}},
		{name: "verify false with key", verification: &PackageSignatureVerification{Verify: &falseValue, PublicKey: "public key"}, wantErr: "cannot be combined"},
		{name: "no enabled method", verification: &PackageSignatureVerification{}, wantErr: "exactly one of public_key or keyless"},
		{name: "both methods", verification: &PackageSignatureVerification{PublicKey: "public key", Keyless: validKeyless}, wantErr: "exactly one of public_key or keyless"},
		{name: "conflicting identity", verification: &PackageSignatureVerification{Keyless: &KeylessSignatureVerification{CertificateIdentity: "identity", CertificateIdentityRegexp: ".*", CertificateOIDCIssuer: "issuer"}}, wantErr: "exactly one of certificate_identity"},
		{name: "missing issuer", verification: &PackageSignatureVerification{Keyless: &KeylessSignatureVerification{CertificateIdentity: "identity"}}, wantErr: "exactly one of certificate_oidc_issuer"},
		{name: "invalid identity regexp", verification: &PackageSignatureVerification{Keyless: &KeylessSignatureVerification{CertificateIdentityRegexp: "[", CertificateOIDCIssuer: "issuer"}}, wantErr: "invalid certificate_identity_regexp"},
		{name: "invalid issuer regexp", verification: &PackageSignatureVerification{Keyless: &KeylessSignatureVerification{CertificateIdentity: "identity", CertificateOIDCIssuerRegexp: "["}}, wantErr: "invalid certificate_oidc_issuer_regexp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePackageSignatureVerification("example", tt.verification)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateBundleForCreateScopesSignaturePolicy(t *testing.T) {
	b := &UDSBundle{
		UDS:      UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: Metadata{Name: "test"},
		Packages: []Package{{Name: "pkg", Source: "oci://example.com/pkg:v1"}},
	}

	require.NoError(t, b.Validate(), "consumption-time validation should not require create-only signature policy")
	require.ErrorContains(t, validateBundleForCreate(b), "signature_verification block is required")
}

func TestPackageSignatureVerificationOptions(t *testing.T) {
	falseValue := false
	dir := t.TempDir()

	t.Run("disabled", func(t *testing.T) {
		opts, err := packageSignatureVerificationOptions(&Package{
			Name:                  "legacy",
			SignatureVerification: &PackageSignatureVerification{Verify: &falseValue},
		}, dir, dir)
		require.NoError(t, err)
		assert.Equal(t, layout.VerifyNever, opts.VerificationStrategy)
	})

	t.Run("missing package", func(t *testing.T) {
		_, err := packageSignatureVerificationOptions(nil, dir, dir)
		require.ErrorContains(t, err, "package is required")
	})

	t.Run("public key", func(t *testing.T) {
		opts, err := packageSignatureVerificationOptions(&Package{
			Name:                  "keyed",
			SignatureVerification: &PackageSignatureVerification{PublicKey: "test public key"},
		}, dir, dir)
		require.NoError(t, err)
		require.NotNil(t, opts.VerifyBlobOptions)
		assert.Equal(t, layout.VerifyAlways, opts.VerificationStrategy)
		assert.Equal(t, dir, opts.VerifyBlobOptions.TempDir)
		contents, err := os.ReadFile(opts.VerifyBlobOptions.Key)
		require.NoError(t, err)
		assert.Equal(t, "test public key", string(contents))
	})

	t.Run("keyless enforces secure defaults", func(t *testing.T) {
		opts, err := packageSignatureVerificationOptions(&Package{
			Name: "keyless",
			SignatureVerification: &PackageSignatureVerification{Keyless: &KeylessSignatureVerification{
				CertificateIdentityRegexp:   "https://example.com/.*",
				CertificateOIDCIssuerRegexp: "https://issuer.example.com/.*",
				TrustedRoot:                 "{\"mediaType\":\"application/vnd.dev.sigstore.trustedroot+json;version=0.1\"}",
				UseSignedTimestamps:         true,
			}},
		}, dir, dir)
		require.NoError(t, err)
		require.NotNil(t, opts.VerifyBlobOptions)
		assert.False(t, opts.VerifyBlobOptions.CommonVerifyOptions.IgnoreTlog)
		assert.False(t, opts.VerifyBlobOptions.CertVerify.IgnoreSCT)
		assert.True(t, opts.VerifyBlobOptions.CommonVerifyOptions.UseSignedTimestamps)
		assert.Equal(t, "https://example.com/.*", opts.VerifyBlobOptions.CertVerify.CertIdentityRegexp)
		assert.Equal(t, "https://issuer.example.com/.*", opts.VerifyBlobOptions.CertVerify.CertOidcIssuerRegexp)
		assert.Equal(t, dir, opts.VerifyBlobOptions.TempDir)
		assert.Equal(t, filepath.Join(dir, "keyless", "trusted-root.json"), opts.VerifyBlobOptions.CommonVerifyOptions.TrustedRootPath)
	})
}
