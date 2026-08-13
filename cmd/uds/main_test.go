// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/mode"
)

func TestRunUsesLegacyByDefault(t *testing.T) {
	unsetEnv(t, mode.FeatureGatesEnv)
	if err := run([]string{"version"}); err != nil {
		t.Fatal(err)
	}
}

func TestRunResolvesNextBeforeLegacyConfiguration(t *testing.T) {
	unsetEnv(t, mode.FeatureGatesEnv)
	t.Setenv("UDS_CONFIG", t.TempDir())
	err := run([]string{"--feature-gates=NextMode=true", "version"})
	if err == nil || !strings.Contains(err.Error(), "NextMode is not available") {
		t.Fatalf("run() error = %v", err)
	}
}

func unsetEnv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "restore")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

func TestRunPropagatesNormalizedGates(t *testing.T) {
	t.Setenv(mode.FeatureGatesEnv, "NextMode=true")
	if err := run([]string{"version", "--feature-gates", "NextMode=false"}); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(mode.FeatureGatesEnv); got != "NextMode=false" {
		t.Fatalf("%s = %q, want %q", mode.FeatureGatesEnv, got, "NextMode=false")
	}
}

func TestNewRootCommandRejectsUnsupportedMode(t *testing.T) {
	if _, err := newRootCommand(mode.Mode("unknown")); err == nil {
		t.Fatal("newRootCommand() returned nil error")
	}
}
