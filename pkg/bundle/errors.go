// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import "fmt"

// errNil returns an error for a nil parameter.
func errNil(name string) error {
	return fmt.Errorf("%s must not be nil", name)
}

// errEmpty returns an error for an empty parameter.
func errEmpty(name string) error {
	return fmt.Errorf("%s must not be empty", name)
}
