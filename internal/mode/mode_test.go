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
		{"implicit environment true", []string{"create"}, "NextMode", Next, "NextMode=true", []string{"create"}},
		{"flag precedence", []string{"create", "--features", "NextMode=false"}, "NextMode=true", Legacy, "NextMode=false", []string{"create"}},
		{"equals form", []string{"--features=NextMode=true", "create"}, "", Next, "NextMode=true", []string{"create"}},
		{"implicit true", []string{"--features=NextMode", "create"}, "", Next, "NextMode=true", []string{"create"}},
		{"Zarf feature", []string{"zarf", "--features=values=false", "version"}, "", Legacy, "NextMode=false,values=false", []string{"zarf", "--features=values=false", "version"}},
		{"Zarf alias feature", []string{"z", "--features=values=false", "version"}, "", Legacy, "NextMode=false,values=false", []string{"z", "--features=values=false", "version"}},
		{"Next tools zarf feature", []string{"tools", "zarf", "version"}, "NextMode=true,values=false", Next, "NextMode=true,values=false", []string{"tools", "zarf", "--features=values=false", "version"}},
		{"Zarf feature skips task arguments", []string{"run", "zarf"}, "values=false", Legacy, "NextMode=false,values=false", []string{"run", "zarf"}},
		{"double dash", []string{"create", "--", "--features=NextMode=true"}, "", Legacy, "NextMode=false", []string{"create", "--", "--features=NextMode=true"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(string) (string, bool) { return tt.env, tt.env != "" }
			got, features, args, err := Resolve(tt.args, lookup)
			if err != nil || got != tt.mode || features.String() != tt.want || !slices.Equal(args, tt.wantArgs) {
				t.Fatalf("Resolve() = %q, %q, %v, %v", got, features.String(), args, err)
			}
		})
	}
}

func TestResolveRejectsInvalidFlagValues(t *testing.T) {
	for _, value := range []string{"", "Other=true", "NextMode=yes", "NextMode=true,NextMode=false", "NextMode=", "=true"} {
		t.Run(value, func(t *testing.T) {
			_, _, _, err := Resolve([]string{"--features=" + value}, func(string) (string, bool) { return "", false })
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
	_, _, _, err := Resolve([]string{"version", "--features"}, func(string) (string, bool) { return "", false })
	if err == nil {
		t.Fatal("Resolve() returned nil error")
	}
}

func TestResolveRejectsRepeatedFeatureOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--features=NextMode=false", "--features", "NextMode=false"},
		{"--features=NextMode=false", "--features", "NextMode=true"},
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
		{"equals form", []string{"--features=NextMode=false", "zarf", "version"}, []string{"zarf", "version"}},
		{"value form", []string{"zarf", "--features", "NextMode=false", "version"}, []string{"zarf", "version"}},
		{"root flags preserved", []string{"--log-level", "debug", "tools", "zarf", "version"}, []string{"--log-level", "debug", "tools", "zarf", "version"}},
		{"double dash", []string{"run", "--", "--features=NextMode=false"}, []string{"run", "--", "--features=NextMode=false"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripBootstrapArgs(tt.args)
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Fatalf("stripBootstrapArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}
