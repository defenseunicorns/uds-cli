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

	securityHub "github.com/defenseunicorns/uds-security-hub/cmd"
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

// uds-security-hub CLI command
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "[ALPHA] Scan a zarf package for vulnerabilities and generate a report.",
	Long:  "[ALPHA] Scan a zarf package for vulnerabilities and generate a report.",
	Run: func(cmd *cobra.Command, _ []string) {
		org, _ := cmd.Flags().GetString("org")
		packageName, _ := cmd.Flags().GetString("package-name")
		tag, _ := cmd.Flags().GetString("tag")
		dockerUsername, _ := cmd.Flags().GetString("docker-username")
		dockerPassword, _ := cmd.Flags().GetString("docker-password")
		outputFile, _ := cmd.Flags().GetString("output-file")

		securityHub.Execute([]string{
			fmt.Sprintf("--org=%s", org),
			fmt.Sprintf("--package-name=%s", packageName),
			fmt.Sprintf("--tag=%s", tag),
			fmt.Sprintf("--docker-username=%s", dockerUsername),
			fmt.Sprintf("--docker-password=%s", dockerPassword),
			fmt.Sprintf("--output-file=%s", outputFile),
		})
	},
}

// addScanCmdFlags adds the scan command flags
func addScanCmdFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP("docker-username", "u", "", "Optional: Docker username for registry access, accepts CSV values")
	cmd.PersistentFlags().StringP("docker-password", "p", "", "Optional: Docker password for registry access, accepts CSV values")
	cmd.PersistentFlags().StringP("org", "o", "defenseunicorns", "Organization name")
	cmd.PersistentFlags().StringP("package-name", "n", "", "Package Name: packages/uds/gitlab-runner")
	cmd.PersistentFlags().StringP("tag", "g", "", "Tag name (e.g.  16.10.0-uds.0-upstream)")
	cmd.PersistentFlags().StringP("output-file", "f", "", "Output file for CSV results")
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
	addScanCmdFlags(scanCmd)
	rootCmd.AddCommand(scanCmd) // uds-security-hub CLI command
}
