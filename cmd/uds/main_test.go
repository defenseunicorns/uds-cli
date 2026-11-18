// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/defenseunicorns/uds-cli/internal/mode"
)

func TestAdaptMaruWaitArgs(t *testing.T) {
	for _, tt := range []struct{ args, want string }{
		{"zarf tools wait-for tcp localhost:8080 0 --timeout 1s", "zarf tools wait-for network tcp localhost:8080 0 --timeout 1s"},
		{"zarf tools wait-for http localhost:8080 200", "zarf tools wait-for network http localhost:8080 200"},
		{"zarf tools wait-for https example.test 200", "zarf tools wait-for network https example.test 200"},
		{"zarf tools wait-for deployment app available -n app", "zarf tools wait-for resource deployment app available -n app"},
		{"zarf tools wait-for storageclass", "zarf tools wait-for resource storageclass"},
		{"zarf tools wait-for network tcp localhost:8080", "zarf tools wait-for network tcp localhost:8080"},
		{"zarf tools wait-for resource deployment app", "zarf tools wait-for resource deployment app"},
		{"zarf tools wait-for --help", "zarf tools wait-for --help"},
		{"zarf tools wait-for", "zarf tools wait-for"},
		{"run zarf tools wait-for tcp", "run zarf tools wait-for tcp"},
		{"version", "version"},
		{"", ""},
	} {
		t.Run(tt.args, func(t *testing.T) {
			t.Parallel()
			args := strings.Fields(tt.args)
			assert.Equal(t, strings.Fields(tt.want), adaptMaruWaitArgs(args))
			assert.Equal(t, strings.Fields(tt.args), args)
		})
	}
}

func TestRunUsesLegacyByDefault(t *testing.T) {
	unsetEnv(t, mode.FeaturesEnv)
	if err := run([]string{"version"}); err != nil {
		t.Fatal(err)
	}
}

func TestRunUsesNextWhenFeatureEnabled(t *testing.T) {
	unsetEnv(t, mode.FeaturesEnv)
	if err := run([]string{"--features=NextMode=true", "version"}); err != nil {
		t.Fatal(err)
	}
}

func TestRunRetainsNonBundleCommandsInNextMode(t *testing.T) {
	unsetEnv(t, mode.FeaturesEnv)
	if err := run([]string{"--features=NextMode=true", "run", "--help"}); err != nil {
		t.Fatal(err)
	}
}

func unsetEnv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "restore")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

func TestRunPropagatesAllNormalizedFeatures(t *testing.T) {
	t.Setenv(mode.FeaturesEnv, "NextMode,PackageMod")
	if err := run([]string{"version"}); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(mode.FeaturesEnv); got != "NextMode=true,PackageMod=true" {
		t.Fatalf("%s = %q, want %q", mode.FeaturesEnv, got, "NextMode=true,PackageMod=true")
	}
}

func TestNewRootCommandRejectsUnsupportedMode(t *testing.T) {
	if _, err := newRootCommand(mode.Mode("unknown"), mode.FeatureSet{}); err == nil {
		t.Fatal("newRootCommand() returned nil error")
	}
}

func TestPackageModDoesNotSelectNextMode(t *testing.T) {
	unsetEnv(t, mode.FeaturesEnv)
	if err := run([]string{"--features=PackageMod", "package", "mod"}); err == nil {
		t.Fatal("run() exposed the Next package command in Legacy mode")
	}
}
