// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/zarf-dev/zarf/src/pkg/state"
)

// ValidateConfig is the single entry point for validating a fully-resolved
// UDSBundleConfig. It runs nil-checks on the structure and then delegates
// field-level checks to focused sub-validators.
//
// Call this once at the boundary where config is produced (e.g. from the
// ConfigResolver). Downstream consumers should trust the config and skip
// re-validation.
func ValidateConfig(cfg *UDSBundleConfig) error {
	if err := validateConfigStructure(cfg); err != nil {
		return err
	}
	return validateOptions(cfg.Options)
}

// validateConfigStructure asserts that cfg and its required sub-structs are non-nil.
func validateConfigStructure(cfg *UDSBundleConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is required (UDSBundleConfig must not be nil): %w", ErrConfigRequired)
	}
	if cfg.Options == nil {
		return fmt.Errorf("config.Options is required (ConfigOptions must not be nil): %w", ErrConfigOptionsRequired)
	}
	return nil
}

// validateLogLevel rejects a non-empty log level that does not parse. An empty
// level is valid and means "use the default" (info).
func validateLogLevel(level string) error {
	if level == "" {
		return nil
	}
	if _, err := logger.ParseLevel(level); err != nil {
		return err
	}
	return nil
}

// validateOptions validates ConfigOptions field invariants.
func validateOptions(opts *ConfigOptions) error {
	if err := validateLogLevel(opts.LogLevel); err != nil {
		return err
	}
	if err := validateConcurrency(opts.Concurrency); err != nil {
		return err
	}
	if err := validateTmpDir(opts.TmpDir); err != nil {
		return err
	}
	return validateZarfInitOptions(opts.ZarfInit)
}

func validateZarfInitOptions(opts *ZarfInitOptions) error {
	if opts == nil {
		return nil
	}

	registryMode := state.RegistryMode(opts.RegistryMode)
	switch registryMode {
	case "", state.RegistryModeNodePort, state.RegistryModeProxy, state.RegistryModeExternal:
	default:
		return fmt.Errorf("registry_mode must be nodeport, proxy, or external: %w", ErrInvalidZarfInitOptions)
	}

	switch state.MutationPolicy(opts.AgentMutationPolicy) {
	case "", state.MutationPolicyAll, state.MutationPolicyLabeled:
	default:
		return fmt.Errorf("agent_mutation_policy must be all or labeled: %w", ErrInvalidZarfInitOptions)
	}

	const maxPort = 65535
	if opts.RegistryPort < 0 || opts.RegistryPort > maxPort {
		return fmt.Errorf("registry_port must be between 0 and %d: %w", maxPort, ErrInvalidZarfInitOptions)
	}
	if opts.InjectorPort < 0 || opts.InjectorPort > maxPort {
		return fmt.Errorf("injector_port must be between 0 and %d: %w", maxPort, ErrInvalidZarfInitOptions)
	}

	if registryMode == state.RegistryModeExternal && opts.RegistryURL == "" {
		return fmt.Errorf("registry_url is required when registry_mode is external: %w", ErrInvalidZarfInitOptions)
	}
	if opts.RegistryURL != "" {
		if opts.RegistryPushUsername == "" || opts.RegistryPushPassword == "" {
			return fmt.Errorf("registry_push_username and registry_push_password are required with registry_url: %w", ErrInvalidZarfInitOptions)
		}
		if registryMode != "" && registryMode != state.RegistryModeExternal {
			return fmt.Errorf("registry_url cannot be combined with an internal registry_mode: %w", ErrInvalidZarfInitOptions)
		}
		if opts.RegistryPort != 0 {
			return fmt.Errorf("registry_url cannot be combined with registry_port: %w", ErrInvalidZarfInitOptions)
		}
		if opts.RegistrySecret != "" {
			return fmt.Errorf("registry_url cannot be combined with registry_secret: %w", ErrInvalidZarfInitOptions)
		}
		if opts.InjectorPort != 0 {
			return fmt.Errorf("registry_url cannot be combined with injector_port: %w", ErrInvalidZarfInitOptions)
		}
		if opts.InjectorImage != "" {
			return fmt.Errorf("registry_url cannot be combined with injector_image: %w", ErrInvalidZarfInitOptions)
		}
	}

	if opts.GitURL != "" && (opts.GitPushUsername == "" || opts.GitPushPassword == "") {
		return fmt.Errorf("git_push_username and git_push_password are required with git_url: %w", ErrInvalidZarfInitOptions)
	}

	if opts.AgentTLSCA == "" && opts.AgentTLSCert == "" && opts.AgentTLSKey == "" {
		return nil
	}
	if opts.AgentTLSCA == "" || opts.AgentTLSCert == "" || opts.AgentTLSKey == "" {
		return fmt.Errorf("%w: agent_tls_ca, agent_tls_cert, and agent_tls_key must be provided together: %w", ErrInvalidZarfInitOptions, ErrInvalidAgentTLS)
	}
	pair, err := tls.X509KeyPair([]byte(opts.AgentTLSCert), []byte(opts.AgentTLSKey))
	if err != nil {
		return fmt.Errorf("%w: agent TLS certificate and key do not match: %w: %w", ErrInvalidZarfInitOptions, ErrInvalidAgentTLS, err)
	}
	if len(pair.Certificate) == 0 {
		return fmt.Errorf("%w: agent TLS certificate is empty: %w", ErrInvalidZarfInitOptions, ErrInvalidAgentTLS)
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return fmt.Errorf("%w: parsing agent TLS certificate: %w: %w", ErrInvalidZarfInitOptions, ErrInvalidAgentTLS, err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(opts.AgentTLSCA)) {
		return fmt.Errorf("%w: parsing agent TLS CA: %w", ErrInvalidZarfInitOptions, ErrInvalidAgentTLS)
	}
	intermediates := x509.NewCertPool()
	for _, cert := range pair.Certificate[1:] {
		intermediate, err := x509.ParseCertificate(cert)
		if err != nil {
			return fmt.Errorf("%w: parsing agent TLS intermediate certificate: %w: %w", ErrInvalidZarfInitOptions, ErrInvalidAgentTLS, err)
		}
		intermediates.AddCert(intermediate)
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, DNSName: state.ZarfAgentHost}); err != nil {
		return fmt.Errorf("%w: verifying agent TLS certificate for the Zarf agent: %w: %w", ErrInvalidZarfInitOptions, ErrInvalidAgentTLS, err)
	}

	return nil
}

// validateConcurrency enforces the [1, MaxConcurrency] range.
func validateConcurrency(concurrency int) error {
	if concurrency < 1 {
		return fmt.Errorf("concurrency must be >= 1, got %d: %w", concurrency, ErrInvalidConcurrency)
	}
	if concurrency > MaxConcurrency {
		return fmt.Errorf("concurrency must be <= %d, got %d: %w", MaxConcurrency, concurrency, ErrInvalidConcurrency)
	}
	return nil
}

// validateTmpDir asserts that, when set, TmpDir refers to an existing directory.
// An empty value is valid and means "use the OS default".
func validateTmpDir(path string) error {
	if path == "" {
		return nil
	}
	st, err := os.Stat(path) //nolint:gosec // user-provided tmp_dir is validated before use
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("tmp_dir: directory does not exist: %s: %w: %w", path, ErrInvalidTemporaryDirectory, err)
		}
		return fmt.Errorf("tmp_dir: failed to stat directory %s: %w: %w", path, ErrInvalidTemporaryDirectory, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("tmp_dir: path is not a directory: %s: %w", path, ErrInvalidTemporaryDirectory)
	}
	return nil
}
