// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package cmd

import (
	"os"
	"runtime/debug"

	runnerCLI "github.com/defenseunicorns/maru-runner/src/cmd"
	runnerConfig "github.com/defenseunicorns/maru-runner/src/config"

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
	Run: func(_ *cobra.Command, _ []string) {
		os.Args = os.Args[1:]          // grab 'run' and onward from the CLI args
		runnerConfig.CmdPrefix = "uds" // use vendored Zarf inside the runner
		runnerConfig.EnvPrefix = "uds"
		// The maru runner init gets called before the uds-cli init, which looks for RUN_ARCHITECTURE because the EnvPrefix
		// that we set above is not called yet. So in order to set the architecture if passing in UDS_ARCHITECTURE we must set it here.
		archValue := os.Getenv("UDS_ARCHITECTURE")
		if archValue != "" {
			runnerConfig.CLIArch = archValue
		}
		runnerCLI.RootCmd().SetArgs(os.Args)
		runnerCLI.Execute()
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
