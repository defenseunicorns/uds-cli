// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package utils

import (
	"os"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/signing"
)

func Test_IsRegistryURL(t *testing.T) {
	tests := []struct {
		name        string
		description string
		output      string
		wantResult  bool
	}{
		{
			name:        "HasScheme",
			description: "Output has a scheme ://",
			output:      "oci://ghcr.io/defenseunicorns/dev",
			wantResult:  true,
		},
		{
			name:        "HasDomain",
			description: "Output has no scheme but has domain",
			output:      "ghcr.io/defenseunicorns/dev",
			wantResult:  true,
		},
		{
			name:        "HasMultiDomain",
			description: "Output has no scheme but has domain in form of example.example.com",
			output:      "registry.example.io/defenseunicorns/dev",
			wantResult:  true,
		},
		{
			name:        "HasDomainAndNoPath",
			description: "Output has no scheme but has domain in form of example.example.com",
			output:      "registry.example.io",
			wantResult:  true,
		},
		{
			name:        "HasPort",
			description: "Output has no scheme or domain (with .) but has port",
			output:      "localhost:31999",
			wantResult:  true,
		},
		{
			name:        "HasPortWithTrailingSlash",
			description: "Output has no scheme or domain (with .) but has port with trailing /",
			output:      "localhost:31999/path",
			wantResult:  true,
		},
		{
			name:        "IsLocalPath",
			description: "Output is to local path",
			output:      "local/path",
			wantResult:  false,
		},
		{
			name:        "IsCurrentDirectory",
			description: "Output is current directory",
			output:      ".",
			wantResult:  false,
		},
		{
			name:        "IsHiddenDirectory",
			description: "Output is a hidden directory",
			output:      ".dev",
			wantResult:  false,
		},
		{
			name:        "IsHiddenDirectoryWithSlashPrefix",
			description: "Output is a hidden directory nested in path",
			output:      "/pathto/.dev",
			wantResult:  false,
		},
		{
			name:        "HasRareDotInLocalDirectoryPath",
			description: "Output has a rare dot in local directory path",
			output:      "/pathto/test.dev/",
			wantResult:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRegistryURL(tt.output)
			require.Equal(t, tt.wantResult, result, tt.description)
		})
	}
}

func TestVerifyBlobOptionsFromKey(t *testing.T) {
	tests := []struct {
		name    string
		keyPath string
		wantNil bool
	}{
		{name: "empty key path returns nil", keyPath: "", wantNil: true},
		{name: "non-empty key path sets Key", keyPath: "/path/to/key.pub"},
		{name: "any non-empty string sets Key", keyPath: "mykey"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := VerifyBlobOptionsFromKey(tt.keyPath)
			if tt.wantNil {
				require.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.Equal(t, tt.keyPath, result.Key)

			// Verify that other fields are set to their default values
			defaults := signing.DefaultVerifyBlobOptions()
			defaults.Key = tt.keyPath
			require.Equal(t, defaults, *result)
		})
	}
}

func TestBuildVerifyBlobOptions(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name                string
		pkg                 types.Package
		wantNil             bool
		wantErr             string
		wantKey             bool
		wantIgnoreTlog      bool
		wantSignedTS        bool
		wantCertIdentity    string
		wantOIDCIssuer      string
		wantTrustedRootPath bool
	}{
		{
			name:    "no signing config returns nil",
			pkg:     types.Package{},
			wantNil: true,
		},
		{
			name:    "publicKey returns key-based opts with IgnoreTlog true",
			pkg:     types.Package{PublicKey: "fake-key-content"},
			wantKey: true,
			// key-based uses DefaultVerifyBlobOptions which has IgnoreTlog=true
			wantIgnoreTlog: true,
		},
		{
			name: "certificateIdentity sets keyless opts with IgnoreTlog false",
			pkg: types.Package{
				CertificateIdentity:   "https://github.com/org/repo/.github/workflows/release.yml@refs/heads/main",
				CertificateOIDCIssuer: "https://token.actions.githubusercontent.com",
			},
			wantCertIdentity: "https://github.com/org/repo/.github/workflows/release.yml@refs/heads/main",
			wantOIDCIssuer:   "https://token.actions.githubusercontent.com",
			wantIgnoreTlog:   false,
		},
		{
			name: "insecureIgnoreTlog true sets IgnoreTlog true",
			pkg: types.Package{
				CertificateIdentity:   "https://example.com/workflow",
				CertificateOIDCIssuer: "https://token.actions.githubusercontent.com",
				InsecureIgnoreTlog:        true,
			},
			wantIgnoreTlog: true,
		},
		{
			name: "useSignedTimestamps sets flag on opts",
			pkg: types.Package{
				CertificateIdentity:   "https://example.com/workflow",
				CertificateOIDCIssuer: "https://token.actions.githubusercontent.com",
				UseSignedTimestamps:   true,
			},
			wantSignedTS: true,
		},
		{
			name: "trustedRoot writes file and sets TrustedRootPath",
			pkg: types.Package{
				CertificateIdentity:   "https://example.com/workflow",
				CertificateOIDCIssuer: "https://token.actions.githubusercontent.com",
				TrustedRoot:           `{"mediaType":"application/vnd.dev.sigstore.trustedroot+json"}`,
			},
			wantTrustedRootPath: true,
		},
		{
			name: "publicKey and keyless fields are mutually exclusive",
			pkg: types.Package{
				PublicKey:             "fake-key-content",
				CertificateIdentity:   "https://example.com/workflow",
				CertificateOIDCIssuer: "https://token.actions.githubusercontent.com",
			},
			wantErr: "cannot use publicKey together with keyless verification options",
		},
		{
			name: "publicKey and insecureIgnoreTlog are mutually exclusive",
			pkg: types.Package{
				PublicKey:      "fake-key-content",
				InsecureIgnoreTlog: true,
			},
			wantErr: "cannot use publicKey together with keyless verification options",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BuildVerifyBlobOptions(tt.pkg, tmpDir)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			if tt.wantNil {
				require.Nil(t, result)
				return
			}
			require.NotNil(t, result)

			if tt.wantKey {
				require.NotEmpty(t, result.Key)
				keyContent, err := os.ReadFile(result.Key)
				require.NoError(t, err)
				require.Equal(t, tt.pkg.PublicKey, string(keyContent))
			}
			require.Equal(t, tt.wantIgnoreTlog, result.CommonVerifyOptions.IgnoreTlog)
			require.Equal(t, tt.wantSignedTS, result.CommonVerifyOptions.UseSignedTimestamps)
			if tt.wantCertIdentity != "" {
				require.Equal(t, tt.wantCertIdentity, result.CertVerify.CertIdentity)
			}
			if tt.wantOIDCIssuer != "" {
				require.Equal(t, tt.wantOIDCIssuer, result.CertVerify.CertOidcIssuer)
			}
			if tt.wantTrustedRootPath {
				require.NotEmpty(t, result.CommonVerifyOptions.TrustedRootPath)
				rootContent, err := os.ReadFile(result.CommonVerifyOptions.TrustedRootPath)
				require.NoError(t, err)
				require.Equal(t, tt.pkg.TrustedRoot, string(rootContent))
			}
		})
	}
}

func TestValidateVerifyBlobConfig(t *testing.T) {
	tests := []struct {
		name    string
		pkg     types.Package
		wantErr string
	}{
		{
			name: "no signing config is valid",
			pkg:  types.Package{},
		},
		{
			name: "publicKey only is valid",
			pkg:  types.Package{PublicKey: "fake-key"},
		},
		{
			name: "keyless only is valid",
			pkg: types.Package{
				CertificateIdentity:   "https://example.com/workflow",
				CertificateOIDCIssuer: "https://token.actions.githubusercontent.com",
			},
		},
		{
			name: "publicKey and certificateIdentity are mutually exclusive",
			pkg: types.Package{
				PublicKey:             "fake-key",
				CertificateIdentity:   "https://example.com/workflow",
				CertificateOIDCIssuer: "https://token.actions.githubusercontent.com",
			},
			wantErr: "cannot use publicKey together with keyless verification options",
		},
		{
			name: "publicKey and insecureIgnoreTlog are mutually exclusive",
			pkg: types.Package{
				PublicKey:      "fake-key",
				InsecureIgnoreTlog: true,
			},
			wantErr: "cannot use publicKey together with keyless verification options",
		},
		{
			name: "publicKey and trustedRoot are mutually exclusive",
			pkg: types.Package{
				PublicKey:   "fake-key",
				TrustedRoot: `{"mediaType":"application/vnd.dev.sigstore.trustedroot+json"}`,
			},
			wantErr: "cannot use publicKey together with keyless verification options",
		},
		{
			name:    "trustedRoot alone requires identity and issuer",
			pkg:     types.Package{TrustedRoot: `{"mediaType":"application/vnd.dev.sigstore.trustedroot+json"}`},
			wantErr: "keyless verification requires certificateIdentity or certificateIdentityRegexp",
		},
		{
			name:    "insecureIgnoreTlog alone requires identity and issuer",
			pkg:     types.Package{InsecureIgnoreTlog: true},
			wantErr: "keyless verification requires certificateIdentity or certificateIdentityRegexp",
		},
		{
			name:    "useSignedTimestamps alone requires identity and issuer",
			pkg:     types.Package{UseSignedTimestamps: true},
			wantErr: "keyless verification requires certificateIdentity or certificateIdentityRegexp",
		},
		{
			name: "keyless with identity but missing issuer",
			pkg: types.Package{
				CertificateIdentity: "https://example.com/workflow",
			},
			wantErr: "keyless verification requires certificateOIDCIssuer or certificateOIDCIssuerRegexp",
		},
		{
			name: "keyless with issuer but missing identity",
			pkg: types.Package{
				CertificateOIDCIssuer: "https://token.actions.githubusercontent.com",
			},
			wantErr: "keyless verification requires certificateIdentity or certificateIdentityRegexp",
		},
		{
			name: "certificateIdentity and certificateIdentityRegexp are mutually exclusive",
			pkg: types.Package{
				CertificateIdentity:       "https://example.com/workflow",
				CertificateIdentityRegexp: "https://.*",
				CertificateOIDCIssuer:     "https://token.actions.githubusercontent.com",
			},
			wantErr: "certificateIdentity and certificateIdentityRegexp are mutually exclusive",
		},
		{
			name: "certificateOIDCIssuer and certificateOIDCIssuerRegexp are mutually exclusive",
			pkg: types.Package{
				CertificateIdentity:         "https://example.com/workflow",
				CertificateOIDCIssuer:       "https://token.actions.githubusercontent.com",
				CertificateOIDCIssuerRegexp: "https://.*",
			},
			wantErr: "certificateOIDCIssuer and certificateOIDCIssuerRegexp are mutually exclusive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVerifyBlobConfig(tt.pkg)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestResolveVerifyBlobOptions(t *testing.T) {
	customOpts := signing.VerifyBlobOptions{}
	customOpts.Key = "/path/to/key.pub"

	tests := []struct {
		name string
		opts *signing.VerifyBlobOptions
		want signing.VerifyBlobOptions
	}{
		{
			name: "nil input returns defaults",
			opts: nil,
			want: signing.DefaultVerifyBlobOptions(),
		},
		{
			name: "non-nil input returned as-is",
			opts: &customOpts,
			want: customOpts,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveVerifyBlobOptions(tt.opts)
			require.Equal(t, tt.want, result)
		})
	}
}
