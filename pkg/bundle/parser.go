// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"

	"github.com/defenseunicorns/uds-cli/internal/bundlehcl"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

const (
	// BundleFileName is the standard bundle definition filename.
	BundleFileName = bundlehcl.BundleFileName
	// BundleDefaultsFileName is the standard bundle defaults filename.
	BundleDefaultsFileName = bundlehcl.BundleDefaultsFileName
	// MaxConcurrency is the maximum supported package deployment concurrency.
	MaxConcurrency = bundlehcl.MaxConcurrency
)

// ResolveBundlePath resolves a bundle directory or definition path.
func ResolveBundlePath(ref string) string {
	return bundlehcl.ResolveBundlePath(ref)
}

// ParseDefaults parses variables from a bundle defaults file.
func ParseDefaults(ctx context.Context, path string) (Variables, error) {
	variables, err := bundlehcl.ParseDefaults(ctx, path)
	return fromInternalVariables(variables), err
}

// MergeVariables recursively merges variable maps without mutating either input.
func MergeVariables(base, overrides Variables) Variables {
	return fromInternalVariables(bundlehcl.MergeVariables(toInternalVariables(base), toInternalVariables(overrides)))
}

// HCLParser exposes bundle and configuration parsing through the public facade.
type HCLParser struct {
	parser *bundlehcl.HCLParser
}

// NewHCLParser creates an HCL parser for the target architecture.
func NewHCLParser(arch string, streams iostreams.IOStreams) *HCLParser {
	return &HCLParser{parser: bundlehcl.NewHCLParser(arch, streams)}
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
func toInternalConfig(cfg *UDSBundleConfig) *bundlehcl.UDSBundleConfig {
	if cfg == nil {
		return nil
	}

	var global *bundlehcl.GlobalOptions
	if cfg.Global != nil {
		global = &bundlehcl.GlobalOptions{LogLevel: cfg.Global.LogLevel, Prompt: cfg.Global.Prompt}
	}
	var options *bundlehcl.ConfigOptions
	if cfg.Options != nil {
		options = &bundlehcl.ConfigOptions{
			LogLevel:      cfg.Options.LogLevel,
			Architecture:  cfg.Options.Architecture,
			PlainHTTP:     cfg.Options.PlainHTTP,
			SkipTLSVerify: cfg.Options.SkipTLSVerify,
			UDSCache:      cfg.Options.UDSCache,
			TmpDir:        cfg.Options.TmpDir,
			Concurrency:   cfg.Options.Concurrency,
		}
	}
	return &bundlehcl.UDSBundleConfig{
		Global:    global,
		Options:   options,
		Variables: toInternalVariables(cfg.Variables),
		Remain:    cfg.Remain,
	}
}

// toInternalConfigOptions converts public configuration options to internal options.
func toInternalConfigOptions(opts ConfigOptions) bundlehcl.ConfigOptions {
	return bundlehcl.ConfigOptions{
		LogLevel: opts.LogLevel, Architecture: opts.Architecture,
		PlainHTTP: opts.PlainHTTP, SkipTLSVerify: opts.SkipTLSVerify,
		UDSCache: opts.UDSCache, TmpDir: opts.TmpDir, Concurrency: opts.Concurrency,
	}
}

// fromInternalConfig converts internal HCL configuration to the public representation.
func fromInternalConfig(cfg *bundlehcl.UDSBundleConfig) *UDSBundleConfig {
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
		Global:    global,
		Options:   options,
		Variables: fromInternalVariables(cfg.Variables),
		Remain:    cfg.Remain,
	}
}

// toInternalVariables recursively converts public variables to internal variables.
func toInternalVariables(variables Variables) bundlehcl.Variables {
	if variables == nil {
		return nil
	}
	converted := make(bundlehcl.Variables, len(variables))
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
func fromInternalVariables(variables bundlehcl.Variables) Variables {
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
	case bundlehcl.Variables:
		return fromInternalVariables(value)
	case map[string]any:
		return fromInternalVariables(bundlehcl.Variables(value))
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
