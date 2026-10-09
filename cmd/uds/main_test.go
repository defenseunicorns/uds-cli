// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package main

import (
	"os"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/mode"
)

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
