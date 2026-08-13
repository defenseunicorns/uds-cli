// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"testing"

	runnerConfig "github.com/defenseunicorns/maru-runner/src/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
)

func TestConfigureRunnerEnvironmentUsesArchitectureEnvironment(t *testing.T) {
	originalArch := config.CLIArch
	config.CLIArch = ""
	t.Cleanup(func() {
		config.CLIArch = originalArch
		runnerConfig.ClearExtraEnv()
	})
	t.Setenv("UDS_ARCHITECTURE", "arm64")
	runnerConfig.ClearExtraEnv()

	configureRunnerEnvironment()

	if got := runnerConfig.GetExtraEnv()["UDS_ARCH"]; got != "arm64" {
		t.Fatalf("UDS_ARCH = %q, want arm64", got)
	}
}
