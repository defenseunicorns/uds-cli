// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarfexec

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
)

func TestExecuteRestoresProcessArguments(t *testing.T) {
	originalArgs := append([]string(nil), os.Args...)
	wantArgs := []string{"zarf", "version"}
	wantError := errors.New("test error")

	err := execute(context.Background(), []string{"version"}, func(context.Context) error {
		if !slices.Equal(os.Args, wantArgs) {
			t.Fatalf("os.Args = %q, want %q", os.Args, wantArgs)
		}
		return wantError
	})

	if !errors.Is(err, wantError) {
		t.Fatalf("execute() error = %v, want %v", err, wantError)
	}
	if !slices.Equal(os.Args, originalArgs) {
		t.Fatalf("os.Args = %q, want restored %q", os.Args, originalArgs)
	}
}
