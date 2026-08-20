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

func unsetEnv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "restore")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

func TestRunPropagatesNormalizedFeatures(t *testing.T) {
	t.Setenv(mode.FeaturesEnv, "NextMode=true")
	if err := run([]string{"version", "--features", "NextMode=false"}); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(mode.FeaturesEnv); got != "NextMode=false" {
		t.Fatalf("%s = %q, want %q", mode.FeaturesEnv, got, "NextMode=false")
	}
}

func TestNewRootCommandRejectsUnsupportedMode(t *testing.T) {
	if _, err := newRootCommand(mode.Mode("unknown")); err == nil {
		t.Fatal("newRootCommand() returned nil error")
	}
}
