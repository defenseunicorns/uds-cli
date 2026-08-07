// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrs_EmptyIsNil(t *testing.T) {
	t.Parallel()

	e := newErrs()
	assert.Equal(t, 0, e.Len())
	assert.NoError(t, e.Err(), "fresh accumulator should report no error")
}

func TestErrs_AddNilIsNoop(t *testing.T) {
	t.Parallel()

	// Add(nil) must not grow the accumulator. This contract lets callers
	// forward a function's error result directly without a nil-guard.
	e := newErrs()
	e.Add(nil)
	e.Add(nil)
	assert.Equal(t, 0, e.Len())
	assert.NoError(t, e.Err())
}

func TestErrs_SingleErrorRoundtrips(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	e := newErrs()
	e.Add(want)

	assert.Equal(t, 1, e.Len())
	require.ErrorIs(t, e.Err(), want, "single error must round-trip via errors.Is")
}

func TestErrs_MultipleErrorsAreJoined(t *testing.T) {
	t.Parallel()

	// All non-nil errors must be reachable via errors.Is on the joined
	// result; this is the property the orchestrator relies on so callers
	// see every per-package failure, not just the first.
	a := errors.New("a")
	b := errors.New("b")
	e := newErrs()
	e.Add(a)
	e.Add(b)

	assert.Equal(t, 2, e.Len())
	got := e.Err()
	require.Error(t, got)
	require.ErrorIs(t, got, a)
	require.ErrorIs(t, got, b)
}

func TestErrs_NilMixedWithRealErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("real")
	e := newErrs()
	e.Add(nil)
	e.Add(want)
	e.Add(nil)

	assert.Equal(t, 1, e.Len())
	require.ErrorIs(t, e.Err(), want)
}

func TestErrs_ConcurrentAddsAreRaceFree(t *testing.T) {
	t.Parallel()

	// Run with -race to exercise the mutex. Every goroutine appends a
	// distinct error; final count must equal the goroutine count, with no
	// races and no lost updates.
	const goroutines = 100
	e := newErrs()

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			e.Add(errors.New("err"))
		}()
	}
	wg.Wait()

	assert.Equal(t, goroutines, e.Len())
}

func TestErrs_ErrIsSafeDuringConcurrentAdds(t *testing.T) {
	t.Parallel()

	// Err must be readable while Add is in flight; the contract is "safe
	// for concurrent use", not "safe only after all Adds returned".
	e := newErrs()
	const adders = 50

	var wg sync.WaitGroup
	wg.Add(adders)
	for range adders {
		go func() {
			defer wg.Done()
			e.Add(errors.New("err"))
		}()
	}
	// Read while writes are happening; the only assertion is that Err does
	// not panic or race. The final count is checked after Wait.
	_ = e.Err()
	wg.Wait()

	assert.Equal(t, adders, e.Len())
}
