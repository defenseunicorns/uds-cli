// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import "fmt"

// errEmpty reports a required configuration value that was empty.
func errEmpty(name string) error {
	return fmt.Errorf("%s must not be empty", name)
}
