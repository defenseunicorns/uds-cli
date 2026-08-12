// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// validSuffix enforces that the suffix is safe for use in OCI tags, filenames,
// and HCL string content. Must start with '-' and contain only safe characters.
var validSuffix = regexp.MustCompile(`^-[a-zA-Z0-9._-]+$`)

// ValidateConfig validates a resolved public bundle configuration.
func ValidateConfig(cfg *UDSBundleConfig) error {
	return bundleinternal.ValidateConfig(toInternalConfig(cfg))
}

// ValidatePackageNames checks that every requested package exists in packages.
func ValidatePackageNames(names []string, packages []spec.Package) error {
	return bundleinternal.ValidatePackageNames(names, packages)
}

func validateLogLevel(level string) error {
	if level == "" {
		return nil
	}
	_, err := logger.ParseLevel(level)
	return err
}

// Validate checks that DeployOptions is valid. Config must be non-nil and valid;
// at least one of BundlePath or Bundle must be provided.
func (o DeployOptions) Validate() error {
	if err := ValidateConfig(o.Config); err != nil {
		return err
	}
	if o.BundlePath == "" && o.Bundle == nil {
		return fmt.Errorf("at least one of BundlePath or Bundle must be provided")
	}
	return nil
}

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

// Validate checks that LoadOptions is valid.
func (o LoadOptions) Validate() error {
	return nil
}

// Validate checks that RemoveOptions is valid. Config must be non-nil and valid;
// at least one of BundlePath or Bundle must be provided.
func (o RemoveOptions) Validate() error {
	if err := ValidateConfig(o.Config); err != nil {
		return err
	}
	if o.BundlePath == "" && o.Bundle == nil {
		return fmt.Errorf("at least one of BundlePath or Bundle must be provided")
	}
	return nil
}

// Validate checks that RemovePackageOptions is valid.
func (o RemovePackageOptions) Validate() error {
	return ValidateConfig(o.Config)
}

// Validate checks that CreateOptions is valid.
func (o CreateOptions) Validate() error {
	if err := ValidateConfig(o.Config); err != nil {
		return err
	}
	if o.BundleFile == "" {
		return fmt.Errorf("BundleFile is required")
	}
	return nil
}

// Validate checks that PullOptions is valid.
func (o PullOptions) Validate() error {
	return ValidateConfig(o.Config)
}

// Validate checks the inspect source and configuration.
func (o InspectOptions) Validate() error {
	if err := ValidateConfig(o.Config); err != nil {
		return err
	}
	if strings.TrimSpace(o.Source) == "" {
		return fmt.Errorf("source must not be empty")
	}

	if IsOCIReference(o.Source) {
		_, err := udsoci.ReferenceIdentifier(o.Source)
		return err
	}
	if !IsTarZst(o.Source) {
		return fmt.Errorf("source must be a .tar.zst bundle artifact or OCI reference")
	}
	return nil
}

// Validate checks that PushOptions is valid.
func (o PushOptions) Validate() error {
	return ValidateConfig(o.Config)
}

// Validate checks that ReconfigureOptions is valid.
// Checks Source, DefaultsFile, and Suffix shape.
func (o ReconfigureOptions) Validate() error {
	if o.Source == "" {
		return fmt.Errorf("source is required")
	}
	if o.DefaultsFile == "" {
		return fmt.Errorf("defaults file is required")
	}
	if !validSuffix.MatchString(o.Suffix) {
		return fmt.Errorf("invalid suffix %q: must start with '-' and contain only alphanumeric characters, dots, underscores, and hyphens", o.Suffix)
	}
	if err := validateLogLevel(o.Options.LogLevel); err != nil {
		return err
	}
	return nil
}

// ValidateRemovalSafety returns an error if removing the named packages would
// leave any remaining bundle package with a missing dependency.
func ValidateRemovalSafety(ctx context.Context, streams iostreams.IOStreams, b *spec.UDSBundle, packageNames []string) error {
	violations, err := bundleinternal.RemovalViolations(ctx, streams, b, packageNames)
	if err != nil {
		return err
	}
	if len(violations) == 0 {
		return nil
	}
	return formatDependencyError("cannot remove package(s) with bundle dependents", "is required by", violations)
}

// ValidateDeploySafety returns an error if any selected package depends on a
// package that is not selected.
func ValidateDeploySafety(ctx context.Context, streams iostreams.IOStreams, b *spec.UDSBundle, packageNames []string) error {
	violations, err := bundleinternal.DeployViolations(ctx, streams, b, packageNames)
	if err != nil {
		return err
	}
	if len(violations) == 0 {
		return nil
	}
	return formatDependencyError("cannot deploy package(s) with unselected dependencies", "requires", violations)
}
