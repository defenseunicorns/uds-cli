// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

func TestValidatePackageSignatureVerification(t *testing.T) {
	falseValue := false
	trueValue := true
	validKeyless := &spec.KeylessSignatureVerification{CertificateIdentity: "https://example.com/workflow", CertificateOIDCIssuer: "https://issuer.example.com"}
	tests := []struct {
		name         string
		verification *spec.PackageSignatureVerification
		wantErr      string
	}{
		{name: "missing block", wantErr: "signature_verification block is required"},
		{name: "verify false", verification: &spec.PackageSignatureVerification{Verify: &falseValue}},
		{name: "public key", verification: &spec.PackageSignatureVerification{PublicKey: "public key"}},
		{name: "blank public key", verification: &spec.PackageSignatureVerification{PublicKey: " \t\n", Keyless: validKeyless}, wantErr: "public_key must not be blank"},
		{name: "keyless", verification: &spec.PackageSignatureVerification{Keyless: validKeyless}},
		{name: "verify true public key", verification: &spec.PackageSignatureVerification{Verify: &trueValue, PublicKey: "public key"}},
		{name: "verify false with key", verification: &spec.PackageSignatureVerification{Verify: &falseValue, PublicKey: "public key"}, wantErr: "cannot be combined"},
		{name: "no enabled method", verification: &spec.PackageSignatureVerification{}, wantErr: "exactly one of public_key or keyless"},
		{name: "both methods", verification: &spec.PackageSignatureVerification{PublicKey: "public key", Keyless: validKeyless}, wantErr: "exactly one of public_key or keyless"},
		{name: "conflicting identity", verification: &spec.PackageSignatureVerification{Keyless: &spec.KeylessSignatureVerification{CertificateIdentity: "identity", CertificateIdentityRegexp: ".*", CertificateOIDCIssuer: "issuer"}}, wantErr: "exactly one of certificate_identity"},
		{name: "missing issuer", verification: &spec.PackageSignatureVerification{Keyless: &spec.KeylessSignatureVerification{CertificateIdentity: "identity"}}, wantErr: "exactly one of certificate_oidc_issuer"},
		{name: "invalid identity regexp", verification: &spec.PackageSignatureVerification{Keyless: &spec.KeylessSignatureVerification{CertificateIdentityRegexp: "[", CertificateOIDCIssuer: "issuer"}}, wantErr: "invalid certificate_identity_regexp"},
		{name: "invalid issuer regexp", verification: &spec.PackageSignatureVerification{Keyless: &spec.KeylessSignatureVerification{CertificateIdentity: "identity", CertificateOIDCIssuerRegexp: "["}}, wantErr: "invalid certificate_oidc_issuer_regexp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageSignatureVerification("example", tt.verification)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestPackageSignatureVerificationOptions(t *testing.T) {
	falseValue := false
	dir := t.TempDir()

	t.Run("disabled", func(t *testing.T) {
		opts, err := PackageSignatureVerificationOptions(&spec.Package{Name: "legacy", SignatureVerification: &spec.PackageSignatureVerification{Verify: &falseValue}}, dir, dir)
		require.NoError(t, err)
		assert.Equal(t, layout.VerifyNever, opts.VerificationStrategy)
	})

	t.Run("missing package", func(t *testing.T) {
		_, err := PackageSignatureVerificationOptions(nil, dir, dir)
		require.ErrorContains(t, err, "package is required")
	})

	t.Run("public key", func(t *testing.T) {
		opts, err := PackageSignatureVerificationOptions(&spec.Package{Name: "keyed", SignatureVerification: &spec.PackageSignatureVerification{PublicKey: "test public key"}}, dir, dir)
		require.NoError(t, err)
		require.NotNil(t, opts.VerifyBlobOptions)
		assert.Equal(t, layout.VerifyAlways, opts.VerificationStrategy)
		contents, err := os.ReadFile(opts.VerifyBlobOptions.Key)
		require.NoError(t, err)
		assert.Equal(t, "test public key", string(contents))
	})

	t.Run("keyless enforces secure defaults", func(t *testing.T) {
		opts, err := PackageSignatureVerificationOptions(&spec.Package{
			Name: "keyless",
			SignatureVerification: &spec.PackageSignatureVerification{Keyless: &spec.KeylessSignatureVerification{
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
		assert.Equal(t, filepath.Join(dir, "keyless", "trusted-root.json"), opts.VerifyBlobOptions.CommonVerifyOptions.TrustedRootPath)
	})
}

func writeValidUnsignedZarfPackage(t *testing.T, dir string) {
	t.Helper()
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	zarfYAML := "kind: ZarfPackageConfig\nmetadata:\n  name: test\n  version: 1.0.0\n  aggregateChecksum: " + emptySHA256 + "\ncomponents: []\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte(zarfYAML), tmpFilePerm))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "checksums.txt"), nil, tmpFilePerm))
}
