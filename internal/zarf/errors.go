// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"errors"
	"fmt"
)

// ErrNotImplemented is returned by interface stub methods that have no
// production implementation yet.
var ErrNotImplemented = errors.New("not yet implemented")

// ErrPackageNotDeployed is returned by Remover.RemovePackage when the requested
// package is not present on the target. The orchestrator treats this as a skip
// rather than a failure.
var ErrPackageNotDeployed = errors.New("package not deployed")

// errNil returns an error for a nil parameter.
func errNil(name string) error {
	return fmt.Errorf("%s must not be nil", name)
}

// errEmpty returns an error for an empty parameter.
func errEmpty(name string) error {
	return fmt.Errorf("%s must not be empty", name)
}
