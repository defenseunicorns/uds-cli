// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"errors"

	"github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
)

// ValidateBundleForCreate applies the create-only package signature policy.
func ValidateBundleForCreate(b *spec.UDSBundle) error {
	if b == nil {
		return ErrBundleNil
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
