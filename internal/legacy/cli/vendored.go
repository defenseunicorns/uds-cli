// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"os"
	"runtime/debug"

	runnerCLI "github.com/defenseunicorns/maru-runner/src/cmd"
	runnerConfig "github.com/defenseunicorns/maru-runner/src/config"
	"github.com/defenseunicorns/pkg/exec"
	"github.com/defenseunicorns/uds-cli/internal/legacy/zarfexec"
	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config/lang"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/message"
	"github.com/spf13/cobra"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
)

func newVendoredCommands() (*cobra.Command, *cobra.Command) {
	return NewRunCommand(), newZarfCommand()
}

// NewRunCommand constructs the retained Legacy task runner command.
func NewRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "run",
		Aliases: []string{"r"},
		Short:   lang.CmdRunShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			configureRunnerEnvironment()

			executablePath, err := exec.GetFinalExecutablePath()
			if err != nil {
				return err
			}
			for name, mutation := range map[string]string{
				"uds":     executablePath,
				"zarf":    executablePath + " zarf",
				"kubectl": executablePath + " zarf tools kubectl",
			} {
				if err := exec.RegisterCmdMutation(name, mutation); err != nil {
					return err
				}
			}

			runnerRoot := runnerCLI.RootCmd()
			runnerRoot.SetArgs(append([]string{"run"}, args...))
			if err := runnerRoot.PersistentFlags().Set("log-level", "warn"); err != nil {
				message.Warnf("unable to set log-level: %s", err)
			}
			return runnerRoot.ExecuteContext(cmd.Context())
		},
		DisableFlagParsing: true,
		ValidArgsFunction: func(cmd *cobra.Command, tasks []string, task string) ([]string, cobra.ShellCompDirective) {
			return runnerCLI.ListAutoCompleteTasks(cmd, tasks, task)
		},
	}
}

func configureRunnerEnvironment() {
	runnerConfig.CmdPrefix = "uds"
	runnerConfig.VendorPrefix = "UDS"

	// Maru uses the MARU_ prefix, so add the UDS controls it inherits.
	runnerConfig.AddExtraEnv("UDS_NO_PROGRESS", "true")
	runnerConfig.AddExtraEnv("UDS_ARCH", config.GetArch(os.Getenv("UDS_ARCHITECTURE")))
	runnerConfig.AddExtraEnv(mode.FeaturesEnv, os.Getenv(mode.FeaturesEnv))
}

func newZarfCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "zarf COMMAND",
		Aliases: []string{"z"},
		Short:   lang.CmdZarfShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			setZarfVersion()
			return zarfexec.Execute(cmd.Context(), args)
		},
		DisableFlagParsing: true,
	}
}

func setZarfVersion() {
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range buildInfo.Deps {
			if dep.Path == "github.com/zarf-dev/zarf" {
				zarfConfig.CLIVersion = dep.Version
				return
			}
		}
	}
}
