// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"context"
	"os"
	"runtime/debug"

	runnerCLI "github.com/defenseunicorns/maru-runner/src/cmd"
	runnerConfig "github.com/defenseunicorns/maru-runner/src/config"
	"github.com/defenseunicorns/uds-cli/internal/legacy/zarfexec"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/message"

	"github.com/defenseunicorns/pkg/exec"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config/lang"
	"github.com/spf13/cobra"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
)

func newVendoredCommands() (*cobra.Command, *cobra.Command) {
	// grab Zarf version to make Zarf library checks happy
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range buildInfo.Deps {
			if dep.Path == "github.com/zarf-dev/zarf" {
				zarfConfig.CLIVersion = dep.Version
			}
		}
	}

	runnerCmd := &cobra.Command{
		Use:     "run",
		Aliases: []string{"r"},
		Short:   lang.CmdRunShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			originalStdout := os.Stdout
			os.Stdout = os.Stderr
			defer func() {
				os.Stdout = originalStdout
			}()

			runnerConfig.CmdPrefix = "uds"
			runnerConfig.VendorPrefix = "UDS"

			// Maru by default uses the MARU_ env var prefix - to add any UDS_ env vars we have to add them here
			archValue := config.GetArch(v.GetString(V_ARCHITECTURE))

			// Disable progress bars for ./uds commands
			runnerConfig.AddExtraEnv("UDS_NO_PROGRESS", "true")

			// Add the UDS_ARCH env var to the runner
			runnerConfig.AddExtraEnv("UDS_ARCH", archValue)

			executablePath, err := exec.GetFinalExecutablePath()
			if err != nil {
				return err
			}

			if err = exec.RegisterCmdMutation("uds", executablePath); err != nil {
				return err
			}
			if err = exec.RegisterCmdMutation("zarf", executablePath+" zarf"); err != nil {
				return err
			}
			if err = exec.RegisterCmdMutation("kubectl", executablePath+" zarf tools kubectl"); err != nil {
				return err
			}

			runnerCLI.RootCmd().SetArgs(append([]string{"run"}, args...))
			if err := runnerCLI.RootCmd().PersistentFlags().Set("log-level", "warn"); err != nil {
				message.Warnf("unable to set log-level: %s", err)
			}
			return runnerCLI.RootCmd().ExecuteContext(cmd.Context())
		},
		DisableFlagParsing: true,
		ValidArgsFunction: func(cmd *cobra.Command, tasks []string, task string) ([]string, cobra.ShellCompDirective) {
			return runnerCLI.ListAutoCompleteTasks(cmd, tasks, task)
		},
	}

	zarfCmd := &cobra.Command{
		Use:     "zarf COMMAND",
		Aliases: []string{"z"},
		Short:   lang.CmdZarfShort,
		RunE: func(_ *cobra.Command, args []string) error {
			return zarfexec.Execute(context.TODO(), args)
		},
		DisableFlagParsing: true,
	}

	return runnerCmd, zarfCmd
}
