// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"errors"
	"fmt"

	"oras.land/oras-go/v2/errdef"
)

// ErrBundleSignatureNotFound indicates that a bundle has no signature evidence.
var ErrBundleSignatureNotFound = errors.New("bundle signature evidence not found")

// ErrBundleSignatureDuplicate indicates that a bundle has multiple signature artifacts.
var ErrBundleSignatureDuplicate = errors.New("duplicate bundle signature evidence")

// ErrEmpty returns an error for an empty parameter.
func ErrEmpty(name string) error {
	return fmt.Errorf("%s must not be empty", name)
}

// IsNotFound reports whether err is an ORAS not-found error.
func IsNotFound(err error) bool {
	return errors.Is(err, errdef.ErrNotFound)
}
