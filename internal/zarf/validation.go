// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"fmt"

	"github.com/defenseunicorns/uds-cli/internal/bundlehcl"
)

// Validate checks that DeployPackageOptions is valid.
func (o DeployPackageOptions) Validate() error {
	if err := ValidateConfig(o.Config); err != nil {
		return err
	}
	if o.BundleDir == "" {
		return fmt.Errorf("BundleDir is required")
	}
	return nil
}

// ValidateConfig validates configuration used by the private Zarf integration.
func ValidateConfig(cfg *UDSBundleConfig) error {
	return bundlehcl.ValidateConfig(toBundleHCLConfig(cfg))
}

// toBundleHCLConfig converts Zarf configuration to the shared HCL representation.
func toBundleHCLConfig(cfg *UDSBundleConfig) *bundlehcl.UDSBundleConfig {
	if cfg == nil {
		return nil
	}

	var global *bundlehcl.GlobalOptions
	if cfg.Global != nil {
		global = &bundlehcl.GlobalOptions{
			LogLevel: cfg.Global.LogLevel,
			Prompt:   cfg.Global.Prompt,
		}
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
		Variables: bundlehcl.Variables(cfg.Variables),
		Remain:    cfg.Remain,
	}
}

// Validate checks that RemovePackageOptions is valid.
func (o RemovePackageOptions) Validate() error {
	return ValidateConfig(o.Config)
}
