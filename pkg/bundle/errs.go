// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"errors"
	"sync"
)

// errs is a thread-safe error accumulator. Nothing in the standard library
// or in golang.org/x/sync covers this need:
//
//   - errors.Join (Go 1.20+) builds a joined error from a slice but is just
//     a constructor; concurrent appends to the underlying slice still need
//     a lock.
//   - golang.org/x/sync/errgroup waits on N goroutines but its Wait surfaces
//     only the FIRST non-nil error returned. Sibling failures that happen
//     later are silently dropped, which is exactly the behaviour the deploy
//     orchestrator needs to avoid (every per-package failure must surface).
//
// errs sits on top of errors.Join with a mutex around the slice, giving
// callers a small primitive: Add from any goroutine, Err once at the end.
//
// All methods are safe for concurrent use. Nil errors passed to Add are
// silently dropped so callers do not have to nil-check at every site.
type errs struct {
	mu   sync.Mutex
	list []error
}

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
