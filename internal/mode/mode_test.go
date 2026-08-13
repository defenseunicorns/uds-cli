// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package mode

import (
	"slices"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		env      string
		mode     Mode
		want     string
		wantArgs []string
	}{
		{"default", []string{"create"}, "", Legacy, "NextMode=false", []string{"create"}},
		{"environment", []string{"create"}, "NextMode=true", Next, "NextMode=true", []string{"create"}},
		{"flag precedence", []string{"create", "--feature-gates", "NextMode=false"}, "NextMode=true", Legacy, "NextMode=false", []string{"create"}},
		{"equals form", []string{"--feature-gates=NextMode=true", "create"}, "", Next, "NextMode=true", []string{"create"}},
		{"double dash", []string{"create", "--", "--feature-gates=NextMode=true"}, "", Legacy, "NextMode=false", []string{"create", "--", "--feature-gates=NextMode=true"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(string) (string, bool) { return tt.env, tt.env != "" }
			got, gates, args, err := Resolve(tt.args, lookup)
			if err != nil || got != tt.mode || gates.String() != tt.want || !slices.Equal(args, tt.wantArgs) {
				t.Fatalf("Resolve() = %q, %q, %q, %v", got, gates.String(), args, err)
			}
		})
	}
}

func TestResolveRejectsInvalidFlagValues(t *testing.T) {
	for _, value := range []string{"", "Other=true", "NextMode=yes", "NextMode=true,NextMode=false", "NextMode", "=true"} {
		t.Run(value, func(t *testing.T) {
			_, _, _, err := Resolve([]string{"--feature-gates=" + value}, func(string) (string, bool) { return "", false })
			if err == nil {
				t.Fatal("Resolve() returned nil error")
			}
		})
	}
}

func TestResolveRejectsInvalidEnvironment(t *testing.T) {
	for _, value := range []string{"", "Other=true", "NextMode=yes", "NextMode=false,NextMode=false"} {
		t.Run(value, func(t *testing.T) {
			_, _, _, err := Resolve(nil, func(string) (string, bool) { return value, true })
			if err == nil {
				t.Fatal("Resolve() returned nil error")
			}
		})
	}
}

func TestResolveRejectsMissingFlagValue(t *testing.T) {
	_, _, _, err := Resolve([]string{"version", "--feature-gates"}, func(string) (string, bool) { return "", false })
	if err == nil {
		t.Fatal("Resolve() returned nil error")
	}
}

func TestResolveRejectsRepeatedFeatureGateOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--feature-gates=NextMode=false", "--feature-gates", "NextMode=false"},
		{"--feature-gates=NextMode=false", "--feature-gates", "NextMode=true"},
	} {
		_, _, _, err := Resolve(args, func(string) (string, bool) { return "", false })
		if err == nil {
			t.Fatal("Resolve() returned nil error")
		}
	}
}

func TestStripBootstrapArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"equals form", []string{"--feature-gates=NextMode=false", "zarf", "version"}, []string{"zarf", "version"}},
		{"value form", []string{"zarf", "--feature-gates", "NextMode=false", "version"}, []string{"zarf", "version"}},
		{"double dash", []string{"run", "--", "--feature-gates=NextMode=false"}, []string{"run", "--", "--feature-gates=NextMode=false"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripBootstrapArgs(tt.args)
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Fatalf("stripBootstrapArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}
