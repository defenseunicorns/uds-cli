// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"errors"
	"fmt"

	"github.com/defenseunicorns/uds-cli/internal/bundlehcl"
	"github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
)

// ValidateBundleForCreate applies the create-only package signature policy.
func ValidateBundleForCreate(b *spec.UDSBundle) error {
	if b == nil {
		return fmt.Errorf("bundle must not be nil")
	}
	var errs []error
	if err := b.Validate(); err != nil {
		errs = append(errs, err)
	}
	for i := range b.Packages {
		pkg := &b.Packages[i]
		if err := zarf.ValidatePackageSignatureVerification(pkg.Name, pkg.SignatureVerification); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Validate checks that CreatePackageOptions is valid.
func (o CreatePackageOptions) Validate() error {
	if err := bundlehcl.ValidateConfig(o.Config); err != nil {
		return err
	}
	if o.BlobDir == "" {
		return fmt.Errorf("BlobDir is required")
	}
	if o.BundleDir == "" {
		return fmt.Errorf("BundleDir is required")
	}
	return nil
}
