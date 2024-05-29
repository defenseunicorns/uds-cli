// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package cmd

import (
	"fmt"
	"os"
	"runtime/debug"

	runnerCLI "github.com/defenseunicorns/maru-runner/src/cmd"
	runnerConfig "github.com/defenseunicorns/maru-runner/src/config"
	"github.com/defenseunicorns/pkg/exec"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	zarfCLI "github.com/defenseunicorns/zarf/src/cmd"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/spf13/cobra"
)

var runnerCmd = &cobra.Command{
	Use:     "run",
	Aliases: []string{"r"},
	Short:   lang.CmdRunShort,
	RunE: func(_ *cobra.Command, _ []string) error {
		os.Args = os.Args[1:] // grab 'run' and onward from the CLI args

		runnerConfig.CmdPrefix = "uds"
		runnerConfig.VendorPrefix = "UDS"

		// Maru by default uses the MARU_ env var prefix - to add any UDS_ env vars we have to add them here
		archValue := config.GetArch(v.GetString(V_ARCHITECTURE))
		runnerConfig.AddExtraEnv("UDS", "true")
		runnerConfig.AddExtraEnv("UDS_ARCH", archValue)

		executablePath, err := exec.GetFinalExecutablePath()
		if err != nil {
			return err
		}

		if err = exec.RegisterCmdMutation("uds", executablePath); err != nil {
			return err
		}
		if err = exec.RegisterCmdMutation("zarf", fmt.Sprintf("%s zarf", executablePath)); err != nil {
			return err
		}
		if err = exec.RegisterCmdMutation("kubectl", fmt.Sprintf("%s zarf tools kubectl", executablePath)); err != nil {
			return err
		}

		runnerCLI.RootCmd().SetArgs(os.Args)
		runnerCLI.Execute()

		return nil
	},
	DisableFlagParsing: true,
	ValidArgsFunction: func(cmd *cobra.Command, tasks []string, task string) ([]string, cobra.ShellCompDirective) {
		return runnerCLI.ListAutoCompleteTasks(cmd, tasks, task)
	},
}

var zarfCmd = &cobra.Command{
	Use:     "zarf COMMAND",
	Aliases: []string{"z"},
	Short:   lang.CmdZarfShort,
	Run: func(_ *cobra.Command, _ []string) {
		os.Args = os.Args[1:] // grab 'zarf' and onward from the CLI args
		zarfCLI.Execute()
	},
	DisableFlagParsing: true,
}

func init() {
	// grab Zarf version to make Zarf library checks happy
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range buildInfo.Deps {
			if dep.Path == "github.com/defenseunicorns/zarf" {
				zarfConfig.CLIVersion = dep.Version
			}
		}
	}

	// vendored Zarf command
	if len(os.Args) > 1 && (os.Args[1] == "zarf" || os.Args[1] == "z") && (os.Args[1] == "run" || os.Args[1] == "r") {
		// disable UDS log file for zarf and run commands bc they have their own log file
		config.SkipLogFile = true
	}

	initViper()
	rootCmd.AddCommand(runnerCmd)
	rootCmd.AddCommand(zarfCmd)
}
