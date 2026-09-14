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
		{"Zarf suffix preserved", []string{"zarf", "--features=values=false", "version"}, "NextMode=true", Next, "NextMode=true", []string{"zarf", "--features=values=false", "version"}},
		{"Zarf alias suffix preserved", []string{"z", "tools", "future-tool", "--features", "values=false"}, "NextMode=true", Next, "NextMode=true", []string{"z", "tools", "future-tool", "--features", "values=false"}},
		{"tools zarf suffix preserved", []string{"tools", "zarf", "version", "--features=values=false"}, "NextMode=true", Next, "NextMode=true", []string{"tools", "zarf", "version", "--features=values=false"}},
		{"root features before Zarf", []string{"--features=NextMode=true", "zarf", "--features", "values=false"}, "", Next, "NextMode=true", []string{"zarf", "--features", "values=false"}},
		{"UDS feature skips task arguments", []string{"run", "zarf"}, "NextMode=false", Legacy, "NextMode=false", []string{"run", "zarf"}},
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
	for _, value := range []string{"", "Other=true", "values=true", "NextMode=yes", "NextMode=true,NextMode=false", "NextMode=", "=true"} {
		t.Run(value, func(t *testing.T) {
			_, _, _, err := Resolve([]string{"--features=" + value}, func(string) (string, bool) { return "", false })
			if err == nil {
				t.Fatal("Resolve() returned nil error")
			}
		})
	}
}

func TestResolveRejectsInvalidEnvironment(t *testing.T) {
	for _, value := range []string{"", "Other=true", "values=true", "NextMode=yes", "NextMode=false,NextMode=false"} {
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

func TestPrepareBootstrapArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"equals form", []string{"--features=NextMode=false", "zarf", "version"}, []string{"zarf", "version"}},
		{"value form after Zarf boundary", []string{"zarf", "--features", "NextMode=false", "version"}, []string{"zarf", "--features", "NextMode=false", "version"}},
		{"tools zarf root flags preserved", []string{"--log-level", "debug", "tools", "zarf", "version"}, []string{"--log-level", "debug", "tools", "zarf", "version"}},
		{"root zarf non-tools command preserved", []string{"--log-level", "debug", "zarf", "version"}, []string{"--log-level", "debug", "zarf", "version"}},
		{"root zarf tools strips leading value flag", []string{"--log-level", "debug", "zarf", "tools", "kubectl", "get", "--help"}, []string{"tools", "kubectl", "get", "--help"}},
		{"root zarf tools strips leading bool flag", []string{"--prompt", "zarf", "tools", "kubectl", "get", "--help"}, []string{"tools", "kubectl", "get", "--help"}},
		{"root zarf tools alias strips leading flag", []string{"--log-level", "debug", "zarf", "t", "k", "get", "--help"}, []string{"t", "k", "get", "--help"}},
		{"completion pseudo command", []string{"zarf", "__complete", "tools", "kubectl"}, []string{"__complete", "tools", "kubectl"}},
		{"completion no descriptions pseudo command", []string{"z", "__completeNoDesc", "tools", "kubectl"}, []string{"__completeNoDesc", "tools", "kubectl"}},
		{"double dash", []string{"run", "--", "--features=NextMode=false"}, []string{"run", "--", "--features=NextMode=false"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prepareBootstrapArgs(tt.args)
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Fatalf("prepareBootstrapArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}
