// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package mode

import (
	"slices"
	"testing"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		env       string
		envSet    bool
		wantMode  Mode
		wantArgs  []string
		wantError bool
	}{
		{name: "default", args: []string{"version"}, wantMode: Legacy, wantArgs: []string{"version"}},
		{name: "environment", env: "next", envSet: true, wantMode: Next, wantArgs: []string{}},
		{name: "option equals", args: []string{"--cli-mode=next", "version"}, wantMode: Next, wantArgs: []string{"version"}},
		{name: "option separate", args: []string{"version", "--cli-mode", "next"}, wantMode: Next, wantArgs: []string{"version"}},
		{name: "option precedes environment", args: []string{"--cli-mode=legacy"}, env: "next", envSet: true, wantMode: Legacy, wantArgs: []string{}},
		{name: "boundary", args: []string{"run", "--", "--cli-mode=next"}, wantMode: Legacy, wantArgs: []string{"run", "--", "--cli-mode=next"}},
		{name: "missing value", args: []string{"--cli-mode"}, wantError: true},
		{name: "empty value", args: []string{"--cli-mode="}, wantError: true},
		{name: "unknown option value", args: []string{"--cli-mode=future"}, wantError: true},
		{name: "empty environment value", envSet: true, wantError: true},
		{name: "unknown environment value", env: "future", envSet: true, wantError: true},
		{name: "duplicate", args: []string{"--cli-mode=legacy", "--cli-mode", "legacy"}, wantError: true},
		{name: "conflict", args: []string{"--cli-mode=legacy", "--cli-mode=next"}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			lookupEnv := func(key string) (string, bool) {
				if key == EnvName && test.envSet {
					return test.env, true
				}
				return "", false
			}

			gotMode, gotArgs, err := Resolve(test.args, lookupEnv)
			if test.wantError {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if gotMode != test.wantMode {
				t.Fatalf("Resolve() mode = %q, want %q", gotMode, test.wantMode)
			}
			if !slices.Equal(gotArgs, test.wantArgs) {
				t.Fatalf("Resolve() args = %q, want %q", gotArgs, test.wantArgs)
			}
		})
	}
}
