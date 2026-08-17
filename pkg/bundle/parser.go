// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

const (
	// BundleFileName is the standard bundle definition filename.
	BundleFileName = bundleinternal.BundleFileName
	// BundleDefaultsFileName is the standard bundle defaults filename.
	BundleDefaultsFileName = bundleinternal.BundleDefaultsFileName
	// MaxConcurrency is the maximum supported package deployment concurrency.
	MaxConcurrency = bundleinternal.MaxConcurrency
)

// ResolveBundlePath resolves a bundle directory or definition path.
func ResolveBundlePath(ref string) string {
	return bundleinternal.ResolveBundlePath(ref)
}

// ParseDefaults parses variables from a bundle defaults file.
func ParseDefaults(ctx context.Context, path string) (Variables, error) {
	variables, err := bundleinternal.ParseDefaults(ctx, path)
	return fromInternalVariables(variables), err
}

// MergeVariables recursively merges variable maps without mutating either input.
func MergeVariables(base, overrides Variables) Variables {
	return fromInternalVariables(bundleinternal.MergeVariables(toInternalVariables(base), toInternalVariables(overrides)))
}

// HCLParser exposes bundle and configuration parsing through the public facade.
type HCLParser struct {
	parser *bundleinternal.HCLParser
}

// NewHCLParser creates an HCL parser for the target architecture.
func NewHCLParser(arch string, streams iostreams.IOStreams) *HCLParser {
	return &HCLParser{parser: bundleinternal.NewHCLParser(arch, streams)}
}

// ParseBundleFile parses a bundle definition file into the public semantic model.
func (p *HCLParser) ParseBundleFile(ctx context.Context, filePath string) (*spec.UDSBundle, error) {
	return p.parser.ParseBundleFile(ctx, filePath)
}

// ParseBundleBytes parses an in-memory bundle definition into the public semantic model.
func (p *HCLParser) ParseBundleBytes(ctx context.Context, src []byte) (*spec.UDSBundle, error) {
	return p.parser.ParseBundleBytes(ctx, src)
}

// ParseBundleConfig parses a bundle configuration file.
func (p *HCLParser) ParseBundleConfig(ctx context.Context, filePath string) (*UDSBundleConfig, error) {
	cfg, err := p.parser.ParseBundleConfig(ctx, filePath)
	if err != nil {
		return nil, err
	}
	return fromInternalConfig(cfg), nil
}

// parseAndMaterializeBundleFile parses a bundle and returns its materialized HCL source.
func (p *HCLParser) parseAndMaterializeBundleFile(ctx context.Context, path string) (*spec.UDSBundle, []byte, error) {
	return p.parser.ParseAndMaterializeBundleFile(ctx, path)
}

// toInternalConfig converts public configuration to the internal HCL representation.
func toInternalConfig(cfg *UDSBundleConfig) *bundleinternal.UDSBundleConfig {
	if cfg == nil {
		return nil
	}

	var global *bundleinternal.GlobalOptions
	if cfg.Global != nil {
		global = &bundleinternal.GlobalOptions{LogLevel: cfg.Global.LogLevel, Prompt: cfg.Global.Prompt}
	}
	var options *bundleinternal.ConfigOptions
	if cfg.Options != nil {
		options = &bundleinternal.ConfigOptions{
			LogLevel:      cfg.Options.LogLevel,
			Architecture:  cfg.Options.Architecture,
			PlainHTTP:     cfg.Options.PlainHTTP,
			SkipTLSVerify: cfg.Options.SkipTLSVerify,
			UDSCache:      cfg.Options.UDSCache,
			TmpDir:        cfg.Options.TmpDir,
			Concurrency:   cfg.Options.Concurrency,
		}
	}
	return &bundleinternal.UDSBundleConfig{
		Global:                global,
		Options:               options,
		SignatureVerification: toInternalVerificationPolicy(cfg.SignatureVerification),
		Variables:             toInternalVariables(cfg.Variables),
		Remain:                cfg.Remain,
	}
}

// toInternalConfigOptions converts public configuration options to internal options.
func toInternalConfigOptions(opts ConfigOptions) bundleinternal.ConfigOptions {
	return bundleinternal.ConfigOptions{
		LogLevel: opts.LogLevel, Architecture: opts.Architecture,
		PlainHTTP: opts.PlainHTTP, SkipTLSVerify: opts.SkipTLSVerify,
		UDSCache: opts.UDSCache, TmpDir: opts.TmpDir, Concurrency: opts.Concurrency,
	}
}

// fromInternalConfig converts internal HCL configuration to the public representation.
func fromInternalConfig(cfg *bundleinternal.UDSBundleConfig) *UDSBundleConfig {
	if cfg == nil {
		return nil
	}

	var global *GlobalOptions
	if cfg.Global != nil {
		global = &GlobalOptions{LogLevel: cfg.Global.LogLevel, Prompt: cfg.Global.Prompt}
	}
	var options *ConfigOptions
	if cfg.Options != nil {
		options = &ConfigOptions{
			LogLevel:      cfg.Options.LogLevel,
			Architecture:  cfg.Options.Architecture,
			PlainHTTP:     cfg.Options.PlainHTTP,
			SkipTLSVerify: cfg.Options.SkipTLSVerify,
			UDSCache:      cfg.Options.UDSCache,
			TmpDir:        cfg.Options.TmpDir,
			Concurrency:   cfg.Options.Concurrency,
		}
	}
	return &UDSBundleConfig{
		Global:                global,
		Options:               options,
		SignatureVerification: fromInternalVerificationPolicy(cfg.SignatureVerification),
		Variables:             fromInternalVariables(cfg.Variables),
		Remain:                cfg.Remain,
	}
}

func toInternalVerificationPolicy(policy *VerificationPolicy) *bundleinternal.SignatureVerification {
	if policy == nil {
		return nil
	}
	result := &bundleinternal.SignatureVerification{PublicKey: policy.PublicKey}
	if policy.Keyless != nil {
		result.Keyless = &bundleinternal.KeylessVerification{
			CertificateIdentity: policy.Keyless.CertificateIdentity, CertificateIdentityRegexp: policy.Keyless.CertificateIdentityRegexp,
			CertificateOIDCIssuer: policy.Keyless.CertificateOIDCIssuer, CertificateOIDCIssuerRegexp: policy.Keyless.CertificateOIDCIssuerRegexp,
			TrustedRoot: policy.Keyless.TrustedRoot,
		}
	}
	return result
}

func fromInternalVerificationPolicy(policy *bundleinternal.SignatureVerification) *VerificationPolicy {
	if policy == nil {
		return nil
	}
	result := &VerificationPolicy{PublicKey: policy.PublicKey}
	if policy.Keyless != nil {
		result.Keyless = &KeylessVerification{
			CertificateIdentity: policy.Keyless.CertificateIdentity, CertificateIdentityRegexp: policy.Keyless.CertificateIdentityRegexp,
			CertificateOIDCIssuer: policy.Keyless.CertificateOIDCIssuer, CertificateOIDCIssuerRegexp: policy.Keyless.CertificateOIDCIssuerRegexp,
			TrustedRoot: policy.Keyless.TrustedRoot,
		}
	}
	return result
}

// toInternalVariables recursively converts public variables to internal variables.
func toInternalVariables(variables Variables) bundleinternal.Variables {
	if variables == nil {
		return nil
	}
	converted := make(bundleinternal.Variables, len(variables))
	for key, value := range variables {
		converted[key] = toInternalVariableValue(value)
	}
	return converted
}

// toInternalVariableValue converts nested public variable values to internal values.
func toInternalVariableValue(value any) any {
	switch value := value.(type) {
	case Variables:
		return toInternalVariables(value)
	case map[string]any:
		return toInternalVariables(Variables(value))
	case []any:
		converted := make([]any, len(value))
		for i, item := range value {
			converted[i] = toInternalVariableValue(item)
		}
		return converted
	default:
		return value
	}
}

// fromInternalVariables recursively converts internal variables to public variables.
func fromInternalVariables(variables bundleinternal.Variables) Variables {
	if variables == nil {
		return nil
	}
	converted := make(Variables, len(variables))
	for key, value := range variables {
		converted[key] = fromInternalVariableValue(value)
	}
	return converted
}

// fromInternalVariableValue converts nested internal variable values to public values.
func fromInternalVariableValue(value any) any {
	switch value := value.(type) {
	case bundleinternal.Variables:
		return fromInternalVariables(value)
	case map[string]any:
		return fromInternalVariables(bundleinternal.Variables(value))
	case []any:
		converted := make([]any, len(value))
		for i, item := range value {
			converted[i] = fromInternalVariableValue(item)
		}
		return converted
	default:
		return value
	}
}
