// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"regexp"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// validSuffix enforces that the suffix is safe for use in OCI tags, filenames,
// and HCL string content. Must start with '-' and contain only safe characters.
var validSuffix = regexp.MustCompile(`^-[a-zA-Z0-9._-]+$`)

func validateConfig(cfg *UDSBundleConfig) error {
	if err := bundleinternal.ValidateConfig(toInternalConfig(cfg)); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	return nil
}

func validateOCIReference(ref string) error {
	if _, err := udsoci.ReferenceIdentifier(ref); err != nil {
		return fmt.Errorf("%w %q: %w", ErrInvalidOCIReference, ref, err)
	}
	return nil
}

// Validate checks that DeployOptions is valid. Config must be non-nil and valid.
func (o DeployOptions) Validate() error {
	if err := validateConfig(o.Config); err != nil {
		return err
	}
	return nil
}

// Validate checks that DeployPackageOptions is valid.
func (o DeployPackageOptions) Validate() error {
	if err := validateConfig(o.Config); err != nil {
		return err
	}
	if o.BundleDir == "" {
		return fmt.Errorf("BundleDir is required: %w", ErrBundleDirRequired)
	}
	return nil
}

// Validate checks that RemoveOptions is valid. Config must be non-nil and valid.
func (o RemoveOptions) Validate() error {
	if err := validateConfig(o.Config); err != nil {
		return err
	}
	return nil
}

func (o removePackageOptions) validate() error {
	return validateConfig(o.Config)
}

// Validate checks that CreateOptions is valid.
func (o CreateOptions) Validate() error {
	if err := validateConfig(o.Config); err != nil {
		return err
	}
	return o.Signing.Validate()
}

// Validate checks that PullOptions is valid.
func (o PullOptions) Validate() error {
	return validateConfig(o.Config)
}

// Validate checks that PushOptions is valid.
func (o PushOptions) Validate() error {
	return validateConfig(o.Config)
}

// Validate checks that ReconfigureOptions is valid.
func (o ReconfigureOptions) Validate() error {
	if err := validateConfig(o.Config); err != nil {
		return err
	}
	if !validSuffix.MatchString(o.Suffix) {
		return fmt.Errorf("invalid suffix %q: must start with '-' and contain only alphanumeric characters, dots, underscores, and hyphens: %w", o.Suffix, ErrInvalidSuffix)
	}
	return o.Signing.Validate()
}

func validateRemovalSafety(ctx context.Context, streams iostreams.IOStreams, b *spec.UDSBundle, packageNames []string) error {
	violations, err := bundleinternal.RemovalViolations(ctx, streams, b, packageNames)
	if err != nil {
		return fmt.Errorf("%w for removal from bundle %q: %w", ErrValidateDependencies, b.Metadata.Name, err)
	}
	if len(violations) == 0 {
		return nil
	}
	return formatDependencyError("cannot remove package(s) with bundle dependents", "is required by", violations)
}

func validateDeploySafety(ctx context.Context, streams iostreams.IOStreams, b *spec.UDSBundle, packageNames []string) error {
	violations, err := bundleinternal.DeployViolations(ctx, streams, b, packageNames)
	if err != nil {
		return fmt.Errorf("%w for deployment from bundle %q: %w", ErrValidateDependencies, b.Metadata.Name, err)
	}
	if len(violations) == 0 {
		return nil
	}
	return formatDependencyError("cannot deploy package(s) with unselected dependencies", "requires", violations)
}
