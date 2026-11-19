// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/pki"
	"github.com/zarf-dev/zarf/src/pkg/state"
)

// validBaseConfig returns a fully-valid UDSBundleConfig used as the starting
// point for table-driven validation tests.
func validBaseConfig() *UDSBundleConfig {
	return &UDSBundleConfig{
		Options: &ConfigOptions{LogLevel: "info", Concurrency: 10},
	}
}

func TestValidateConfig_Structure(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *UDSBundleConfig
		wantErr string
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: "config is required",
		},
		{
			name:    "nil Options",
			cfg:     &UDSBundleConfig{},
			wantErr: "config.Options is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	require.NoError(t, ValidateConfig(validBaseConfig()))
}

func TestValidateLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		wantErr string
	}{
		{name: "empty allowed (defaults)", level: ""},
		{name: "debug accepted", level: "debug"},
		{name: "info accepted", level: "info"},
		{name: "warn accepted", level: "warn"},
		{name: "error accepted", level: "error"},
		{name: "invalid rejected", level: "loud", wantErr: "unknown log level"},
		{name: "typo rejected", level: "verbose", wantErr: "unknown log level"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.Options.LogLevel = tt.level
			err := ValidateConfig(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateConcurrency(t *testing.T) {
	tests := []struct {
		name        string
		concurrency int
		wantErr     string
	}{
		{name: "zero rejected", concurrency: 0, wantErr: "concurrency must be >= 1"},
		{name: "negative rejected", concurrency: -1, wantErr: "concurrency must be >= 1"},
		{name: "negative-large rejected", concurrency: -100, wantErr: "concurrency must be >= 1"},
		{name: "above max rejected", concurrency: MaxConcurrency + 1, wantErr: fmt.Sprintf("concurrency must be <= %d", MaxConcurrency)},
		{name: "way above max rejected", concurrency: 9999, wantErr: fmt.Sprintf("concurrency must be <= %d", MaxConcurrency)},
		{name: "lower bound accepted", concurrency: 1},
		{name: "default accepted", concurrency: 10},
		{name: "upper bound accepted", concurrency: MaxConcurrency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.Options.Concurrency = tt.concurrency
			err := ValidateConfig(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateTmpDir(t *testing.T) {
	t.Run("empty allowed", func(t *testing.T) {
		cfg := validBaseConfig()
		cfg.Options.TmpDir = ""
		require.NoError(t, ValidateConfig(cfg))
	})

	t.Run("existing directory accepted", func(t *testing.T) {
		cfg := validBaseConfig()
		cfg.Options.TmpDir = t.TempDir()
		require.NoError(t, ValidateConfig(cfg))
	})

	t.Run("nonexistent path rejected", func(t *testing.T) {
		cfg := validBaseConfig()
		cfg.Options.TmpDir = "/nonexistent/path/tmp"
		err := ValidateConfig(cfg)
		require.ErrorIs(t, err, ErrInvalidTemporaryDirectory)
		require.ErrorIs(t, err, os.ErrNotExist)
		assert.Contains(t, err.Error(), "tmp_dir")
		assert.Contains(t, err.Error(), "directory does not exist")
	})

	t.Run("file rejected", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "afile")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
		cfg := validBaseConfig()
		cfg.Options.TmpDir = f
		err := ValidateConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tmp_dir")
		assert.Contains(t, err.Error(), "path is not a directory")
	})
}

func TestValidateConfig_ZarfInit(t *testing.T) {
	validTLS, err := pki.GeneratePKI(state.ZarfAgentHost)
	require.NoError(t, err)

	tests := []struct {
		name    string
		options *ZarfInitOptions
		wantErr string
	}{
		{name: "omitted"},
		{name: "empty block", options: &ZarfInitOptions{}},
		{
			name: "external services",
			options: &ZarfInitOptions{
				RegistryMode:         "external",
				RegistryURL:          "registry.example.com",
				RegistryPushUsername: "registry-user",
				RegistryPushPassword: "registry-password",
				GitURL:               "https://git.example.com",
				GitPushUsername:      "git-user",
				GitPushPassword:      "git-password",
			},
		},
		{name: "proxy without injector image", options: &ZarfInitOptions{RegistryMode: "proxy"}},
		{name: "valid port boundaries", options: &ZarfInitOptions{RegistryPort: 65535, InjectorPort: 1}},
		{name: "negative registry port", options: &ZarfInitOptions{RegistryPort: -1}, wantErr: "registry_port"},
		{name: "registry port overflow", options: &ZarfInitOptions{RegistryPort: 65536}, wantErr: "registry_port"},
		{name: "negative injector port", options: &ZarfInitOptions{InjectorPort: -1}, wantErr: "injector_port"},
		{name: "injector port overflow", options: &ZarfInitOptions{InjectorPort: 65536}, wantErr: "injector_port"},
		{name: "invalid registry mode", options: &ZarfInitOptions{RegistryMode: "invalid"}, wantErr: "registry_mode"},
		{name: "external mode requires URL", options: &ZarfInitOptions{RegistryMode: "external"}, wantErr: "registry_url"},
		{name: "registry URL requires push user", options: &ZarfInitOptions{RegistryURL: "registry.example.com", RegistryPushPassword: "secret"}, wantErr: "registry_push_username"},
		{name: "registry URL requires push password", options: &ZarfInitOptions{RegistryURL: "registry.example.com", RegistryPushUsername: "user"}, wantErr: "registry_push_password"},
		{name: "registry URL conflicts with nodeport", options: &ZarfInitOptions{RegistryURL: "registry.example.com", RegistryMode: "nodeport", RegistryPushUsername: "user", RegistryPushPassword: "secret"}, wantErr: "registry_mode"},
		{name: "registry URL conflicts with port", options: &ZarfInitOptions{RegistryURL: "registry.example.com", RegistryPort: 5000, RegistryPushUsername: "user", RegistryPushPassword: "secret"}, wantErr: "registry_port"},
		{name: "registry URL conflicts with secret", options: &ZarfInitOptions{RegistryURL: "registry.example.com", RegistrySecret: "secret", RegistryPushUsername: "user", RegistryPushPassword: "password"}, wantErr: "registry_secret"},
		{name: "registry URL conflicts with injector port", options: &ZarfInitOptions{RegistryURL: "registry.example.com", InjectorPort: 5000, RegistryPushUsername: "user", RegistryPushPassword: "secret"}, wantErr: "injector_port"},
		{name: "registry URL conflicts with injector image", options: &ZarfInitOptions{RegistryURL: "registry.example.com", InjectorImage: "injector:v1", RegistryPushUsername: "user", RegistryPushPassword: "secret"}, wantErr: "injector_image"},
		{name: "Git URL requires push credentials", options: &ZarfInitOptions{GitURL: "https://git.example.com"}, wantErr: "git_push_username"},
		{name: "invalid mutation policy", options: &ZarfInitOptions{AgentMutationPolicy: "invalid"}, wantErr: "agent_mutation_policy"},
		{name: "partial agent TLS", options: &ZarfInitOptions{AgentTLSCA: string(validTLS.CA)}, wantErr: "must be provided together"},
		{name: "invalid agent TLS", options: &ZarfInitOptions{AgentTLSCA: "ca", AgentTLSCert: "cert", AgentTLSKey: "key"}, wantErr: "certificate and key"},
		{
			name: "valid agent TLS",
			options: &ZarfInitOptions{
				AgentTLSCA:   string(validTLS.CA),
				AgentTLSCert: string(validTLS.Cert),
				AgentTLSKey:  string(validTLS.Key),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.Options.ZarfInit = tt.options
			err := ValidateConfig(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrInvalidZarfInitOptions)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.NotContains(t, err.Error(), "registry-password")
			assert.NotContains(t, err.Error(), "git-password")
		})
	}
}

func TestValidateConfig_ZarfInitAgentTLSChain(t *testing.T) {
	now := time.Now()
	issue := func(serial int64, name string, isCA bool, parent *x509.Certificate, signer ed25519.PrivateKey) (*x509.Certificate, ed25519.PrivateKey, string) {
		t.Helper()
		pub, key, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		template := &x509.Certificate{
			SerialNumber:          big.NewInt(serial),
			Subject:               pkix.Name{CommonName: name},
			NotBefore:             now.Add(-time.Hour),
			NotAfter:              now.Add(time.Hour),
			BasicConstraintsValid: true,
			IsCA:                  isCA,
		}
		if isCA {
			template.KeyUsage = x509.KeyUsageCertSign
		} else {
			template.DNSNames = []string{name}
			template.KeyUsage = x509.KeyUsageDigitalSignature
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		}
		if parent == nil {
			parent, signer = template, key
		}
		der, err := x509.CreateCertificate(rand.Reader, template, parent, pub, signer)
		require.NoError(t, err)
		cert, err := x509.ParseCertificate(der)
		require.NoError(t, err)
		return cert, key, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	}

	root, rootKey, rootPEM := issue(1, "test root", true, nil, nil)
	issuer, issuerKey, issuerPEM := issue(2, "test intermediate", true, root, rootKey)
	_, leafKey, leafPEM := issue(3, state.ZarfAgentHost, false, issuer, issuerKey)
	_, _, otherRootPEM := issue(4, "unrelated test root", true, nil, nil)
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	require.NoError(t, err)

	cfg := validBaseConfig()
	cfg.Options.ZarfInit = &ZarfInitOptions{
		AgentTLSCA:   rootPEM,
		AgentTLSCert: leafPEM + issuerPEM,
		AgentTLSKey:  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})),
	}
	require.NoError(t, ValidateConfig(cfg))

	cfg.Options.ZarfInit.AgentTLSCert = leafPEM
	require.ErrorIs(t, ValidateConfig(cfg), ErrInvalidAgentTLS)

	cfg.Options.ZarfInit.AgentTLSCert = leafPEM + issuerPEM + rootPEM
	cfg.Options.ZarfInit.AgentTLSCA = otherRootPEM
	require.ErrorIs(t, ValidateConfig(cfg), ErrInvalidAgentTLS)
}

func TestValidateConfig_StopsOnFirstError(t *testing.T) {
	// Confirms ValidateConfig short-circuits: a nil structure should be reported
	// before field-level errors are evaluated.
	err := ValidateConfig(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
	assert.NotContains(t, err.Error(), "concurrency")
}

func TestValidatePackageNames(t *testing.T) {
	packages := []spec.Package{
		{Name: "core"},
		{Name: "nginx"},
		{Name: "podinfo"},
	}

	tests := []struct {
		name            string
		names           []string
		wantErrContains []string
	}{
		{
			name:  "nil names passes",
			names: nil,
		},
		{
			name:  "empty names passes",
			names: []string{},
		},
		{
			name:  "valid single name",
			names: []string{"nginx"},
		},
		{
			name:  "valid multiple names",
			names: []string{"core", "podinfo"},
		},
		{
			name:            "unknown package name includes available packages",
			names:           []string{"bogus"},
			wantErrContains: []string{"unknown packages", "available packages"},
		},
		{
			name:            "mix of valid and invalid",
			names:           []string{"core", "invalid"},
			wantErrContains: []string{"unknown packages", "available packages"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageNames(tt.names, packages)
			if len(tt.wantErrContains) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, s := range tt.wantErrContains {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}
