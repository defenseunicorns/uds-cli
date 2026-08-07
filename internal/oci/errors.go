// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import "fmt"

// ErrEmpty returns an error for an empty parameter.
func ErrEmpty(name string) error {
	return fmt.Errorf("%s must not be empty", name)
}
