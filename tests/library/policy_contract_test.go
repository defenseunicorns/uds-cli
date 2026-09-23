// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/stretchr/testify/require"
)

func TestPublicSigningModeValues(t *testing.T) {
	require.Equal(t, "key", string(bundle.SigningModeKey))
	require.Equal(t, "keyless", string(bundle.SigningModeKeyless))
	require.Equal(t, "unsigned", string(bundle.SigningModeUnsigned))
}

func TestPublicSigningOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		options bundle.SigningOptions
		wantErr error
	}{
		{name: "key", options: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: "private-key"}},
		{name: "keyless", options: bundle.SigningOptions{Mode: bundle.SigningModeKeyless}},
		{name: "unsigned", options: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned}},
		{name: "missing key", options: bundle.SigningOptions{Mode: bundle.SigningModeKey}, wantErr: bundle.ErrInvalidSigningOptions},
		{name: "keyless with key", options: bundle.SigningOptions{Mode: bundle.SigningModeKeyless, Key: "private-key"}, wantErr: bundle.ErrInvalidSigningOptions},
		{name: "unsupported mode", options: bundle.SigningOptions{Mode: bundle.SigningMode("unsupported")}, wantErr: bundle.ErrInvalidSigningOptions},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.Validate()
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestPublicSignOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		options bundle.SignOptions
		wantErr error
	}{
		{
			name: "key signing",
			options: bundle.SignOptions{
				Source:  "bundle.tar.zst",
				Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: "private-key"},
			},
		},
		{
			name: "keyless signing",
			options: bundle.SignOptions{
				Source:  "bundle.tar.zst",
				Signing: bundle.SigningOptions{Mode: bundle.SigningModeKeyless},
			},
		},
		{
			name: "unsigned signing rejected",
			options: bundle.SignOptions{
				Source:  "bundle.tar.zst",
				Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
			},
			wantErr: bundle.ErrInvalidSigningOptions,
		},
		{
			name: "source required",
			options: bundle.SignOptions{
				Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: "private-key"},
			},
			wantErr: bundle.ErrSourceRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.Validate()
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestPublicVerificationPolicyValidate(t *testing.T) {
	tests := []struct {
		name   string
		policy bundle.VerificationPolicy
		valid  bool
	}{
		{
			name:   "public key",
			policy: bundle.VerificationPolicy{PublicKey: "public-key"},
			valid:  true,
		},
		{
			name: "keyless exact constraints",
			policy: bundle.VerificationPolicy{Keyless: &bundle.KeylessVerification{
				CertificateIdentity:   "workflow-identity",
				CertificateOIDCIssuer: "workflow-issuer",
			}},
			valid: true,
		},
		{
			name: "keyless regexp constraints",
			policy: bundle.VerificationPolicy{Keyless: &bundle.KeylessVerification{
				CertificateIdentityRegexp:   `^workflow-identity$`,
				CertificateOIDCIssuerRegexp: `^workflow-issuer$`,
			}},
			valid: true,
		},
		{name: "missing public key and keyless policy", policy: bundle.VerificationPolicy{}},
		{
			name: "public key and keyless policy",
			policy: bundle.VerificationPolicy{PublicKey: "public-key", Keyless: &bundle.KeylessVerification{
				CertificateIdentity:   "workflow-identity",
				CertificateOIDCIssuer: "workflow-issuer",
			}},
		},
		{
			name: "missing identity constraint",
			policy: bundle.VerificationPolicy{Keyless: &bundle.KeylessVerification{
				CertificateOIDCIssuer: "workflow-issuer",
			}},
		},
		{
			name: "missing issuer constraint",
			policy: bundle.VerificationPolicy{Keyless: &bundle.KeylessVerification{
				CertificateIdentity: "workflow-identity",
			}},
		},
		{
			name: "identity exact and regexp",
			policy: bundle.VerificationPolicy{Keyless: &bundle.KeylessVerification{
				CertificateIdentity:         "workflow-identity",
				CertificateIdentityRegexp:   `^workflow-identity$`,
				CertificateOIDCIssuer:       "workflow-issuer",
				CertificateOIDCIssuerRegexp: `^workflow-issuer$`,
			}},
		},
		{
			name: "issuer exact and regexp",
			policy: bundle.VerificationPolicy{Keyless: &bundle.KeylessVerification{
				CertificateIdentityRegexp:   `^workflow-identity$`,
				CertificateOIDCIssuer:       "workflow-issuer",
				CertificateOIDCIssuerRegexp: `^workflow-issuer$`,
			}},
		},
		{
			name: "malformed identity regexp",
			policy: bundle.VerificationPolicy{Keyless: &bundle.KeylessVerification{
				CertificateIdentityRegexp: "[",
				CertificateOIDCIssuer:     "workflow-issuer",
			}},
		},
		{
			name: "malformed issuer regexp",
			policy: bundle.VerificationPolicy{Keyless: &bundle.KeylessVerification{
				CertificateIdentity:         "workflow-identity",
				CertificateOIDCIssuerRegexp: "[",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if tt.valid {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, bundle.ErrInvalidVerificationPolicy)
		})
	}
}

func TestPublicVerifyOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		options bundle.VerifyOptions
		wantErr error
	}{
		{
			name: "public key policy",
			options: bundle.VerifyOptions{
				Source: "bundle.tar.zst",
				Policy: bundle.VerificationPolicy{PublicKey: "public-key"},
			},
		},
		{
			name: "source required",
			options: bundle.VerifyOptions{
				Policy: bundle.VerificationPolicy{PublicKey: "public-key"},
			},
			wantErr: bundle.ErrSourceRequired,
		},
		{
			name:    "invalid policy",
			options: bundle.VerifyOptions{Source: "bundle.tar.zst"},
			wantErr: bundle.ErrInvalidVerificationPolicy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.Validate()
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}
