// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import "errors"

// newErrs returns a ready-to-use, empty accumulator. Safe for concurrent use
// from the moment it is returned.
func newErrs() *errs {
	return &errs{}
}

// Add appends err to the accumulator. Nil is a no-op so callers can pipe an
// error result directly without a guard. Safe for concurrent use.
func (e *errs) Add(err error) {
	if err == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.list = append(e.list, err)
}

// Err returns errors.Join of every non-nil error added so far, or nil if none
// have been collected. Safe for concurrent use, including while Add is in
// flight on other goroutines.
func (e *errs) Err() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return errors.Join(e.list...)
}

// Len reports the number of non-nil errors collected. Useful for assertions
// in tests; production code should prefer Err to read the aggregate value.
// Safe for concurrent use.
func (e *errs) Len() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.list)
}
