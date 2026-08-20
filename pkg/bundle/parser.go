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
	bundleFileName         = bundleinternal.BundleFileName
	bundleDefaultsFileName = bundleinternal.BundleDefaultsFileName
)

func parseBundleFile(ctx context.Context, arch string, streams iostreams.IOStreams, filePath string) (*spec.UDSBundle, error) {
	return bundleinternal.NewHCLParser(arch, streams).ParseBundleFile(ctx, filePath)
}

func parseAndMaterializeBundleFile(ctx context.Context, arch string, streams iostreams.IOStreams, path string) (*spec.UDSBundle, []byte, error) {
	return bundleinternal.NewHCLParser(arch, streams).ParseAndMaterializeBundleFile(ctx, path)
}

// toInternalConfig converts public configuration to the internal HCL representation.
func toInternalConfig(cfg *UDSBundleConfig) *bundleinternal.UDSBundleConfig {
	if cfg == nil {
		return nil
	}

	var options *bundleinternal.ConfigOptions
	if cfg.Options != nil {
		options = &bundleinternal.ConfigOptions{
			LogLevel:      cfg.Options.LogLevel,
			Architecture:  cfg.Options.Architecture,
			PlainHTTP:     cfg.Options.PlainHTTP,
			SkipTLSVerify: cfg.Options.SkipTLSVerify,
			TmpDir:        cfg.Options.TmpDir,
			Concurrency:   cfg.Options.Concurrency,
		}
	}
	return &bundleinternal.UDSBundleConfig{
		Options:               options,
		SignatureVerification: toInternalVerificationPolicy(cfg.SignatureVerification),
		Variables:             toInternalVariables(cfg.Variables),
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

// toInternalConfigOptions converts public configuration options to internal options.
func toInternalConfigOptions(opts ConfigOptions) bundleinternal.ConfigOptions {
	return bundleinternal.ConfigOptions{
		LogLevel: opts.LogLevel, Architecture: opts.Architecture,
		PlainHTTP: opts.PlainHTTP, SkipTLSVerify: opts.SkipTLSVerify,
		TmpDir: opts.TmpDir, Concurrency: opts.Concurrency,
	}
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
