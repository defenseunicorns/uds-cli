// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"fmt"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	zarfapi "github.com/zarf-dev/zarf/src/api"
)

// Validate checks that DeployPackageOptions is valid.
func (o DeployPackageOptions) Validate() error {
	if err := ValidateConfig(o.Config); err != nil {
		return err
	}
	if o.BundleDir == "" {
		return ErrBundleDirRequired
	}
	return nil
}

// ValidateConfig validates configuration used by the private Zarf integration.
func ValidateConfig(cfg *UDSBundleConfig) error {
	return bundleinternal.ValidateConfig(toBundleHCLConfig(cfg))
}

// validatePackageFileChecksums rejects algorithm-only checksums that Zarf v0.88
// otherwise treats as unset and skips during package verification.
func validatePackageFileChecksums(pkg zarfapi.Package) error {
	for _, component := range pkg.Components {
		for _, file := range component.Files {
			if file.Checksum.Algorithm != "" && file.Checksum.Digest == "" {
				return fmt.Errorf("%w: component %q file %q has an empty digest", ErrInvalidPackageFileChecksum, component.Name, file.Source)
			}
		}
	}
	return nil
}

// toBundleHCLConfig converts Zarf configuration to the shared HCL representation.
func toBundleHCLConfig(cfg *UDSBundleConfig) *bundleinternal.UDSBundleConfig {
	if cfg == nil {
		return nil
	}

	return &bundleinternal.UDSBundleConfig{
		Options:   cfg.Options,
		Variables: cfg.Variables,
		Remain:    cfg.Remain,
	}
}

// Validate checks that RemovePackageOptions is valid.
func (o RemovePackageOptions) Validate() error {
	return ValidateConfig(o.Config)
}
