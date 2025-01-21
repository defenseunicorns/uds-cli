// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"bytes"
	"context"
	_ "embed" //embed tofuCLI as a vendored depednecy
	"fmt"
	"io"
	"os"
	osExec "os/exec"
	"runtime/debug"

	runnerCLI "github.com/defenseunicorns/maru-runner/src/cmd"
	runnerConfig "github.com/defenseunicorns/maru-runner/src/config"
	"github.com/zarf-dev/zarf/src/pkg/message"

	"github.com/defenseunicorns/pkg/exec"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/spf13/cobra"
	zarfCLI "github.com/zarf-dev/zarf/src/cmd"
	zarfConfig "github.com/zarf-dev/zarf/src/config"

	securityHub "github.com/defenseunicorns/uds-security-hub/cmd"
)

// NOTE: the bin directory needs to be within the directory or subdirectories of this file at compile time

//go:embed bin/tofu
var tofuCLI []byte

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
		if err = exec.RegisterCmdMutation("zarf", fmt.Sprintf("%s zarf", executablePath)); err != nil {
			return err
		}
		if err = exec.RegisterCmdMutation("kubectl", fmt.Sprintf("%s zarf tools kubectl", executablePath)); err != nil {
			return err
		}

		runnerCLI.RootCmd().SetArgs(os.Args)
		if err := runnerCLI.RootCmd().PersistentFlags().Set("log-level", "warn"); err != nil {
			message.Warnf("unable to set log-level: %s", err)
		}
		runnerCLI.Execute()

		return nil
	},
	DisableFlagParsing: true,
	ValidArgsFunction: func(cmd *cobra.Command, tasks []string, task string) ([]string, cobra.ShellCompDirective) {
		return runnerCLI.ListAutoCompleteTasks(cmd, tasks, task)
	},
}

var zarfCli = &cobra.Command{
	Use:     "zarf COMMAND",
	Aliases: []string{"z"},
	Short:   lang.CmdZarfShort,
	Run: func(_ *cobra.Command, _ []string) {
		os.Args = os.Args[1:] // grab 'zarf' and onward from the CLI args
		zarfCLI.Execute(context.TODO())
	},
	DisableFlagParsing: true,
}

func useEmbeddedTofu() error {
	// Create an executable tofu binary
	tmpTofuBinary, err := os.CreateTemp(config.CommonOptions.TempDirectory, "tofu")
	if err != nil {
		return err
	}
	if err = tmpTofuBinary.Chmod(0755); err != nil {
		return err
	}
	defer os.Remove(tmpTofuBinary.Name())

	if _, err = io.Copy(tmpTofuBinary, io.NopCloser(bytes.NewReader(tofuCLI))); err != nil {
		return err
	}

	// ignore 'uds', grab the high level tofu command ('apply', 'plan', etc.) and onward from the CLI args
	os.Args = os.Args[1:]
	cmd := osExec.Command(tmpTofuBinary.Name(), os.Args...) //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	return err
}

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: lang.CmdBundlePlanShort,
	// Args:  cobra.MaximumNArgs(0),
	RunE: func(_ *cobra.Command, _ []string) error {
		return useEmbeddedTofu()
	},
	DisableFlagParsing: true,
}

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: lang.CmdBundleApplyShort,
	Args:  cobra.MaximumNArgs(0),
	RunE: func(_ *cobra.Command, _ []string) error {
		return useEmbeddedTofu()
	},
	DisableFlagParsing: true,
}

// uds-security-hub CLI command
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "[ALPHA] Scan a zarf package for vulnerabilities and generate a report.",
	Long:  "[ALPHA] Scan a zarf package for vulnerabilities and generate a report.",
	Run: func(_ *cobra.Command, _ []string) {
		os.Args = os.Args[1:] // grab 'scan' and onward from the CLI args
		securityHub.Execute(os.Args)
	},
	DisableFlagParsing: true,
}

func init() {
	// grab Zarf version to make Zarf library checks happy
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range buildInfo.Deps {
			if dep.Path == "github.com/zarf-dev/zarf" {
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
	rootCmd.AddCommand(zarfCli)
	rootCmd.AddCommand(scanCmd)  // uds-security-hub CLI command
	rootCmd.AddCommand(planCmd)  // tofu plan
	rootCmd.AddCommand(applyCmd) // tofu apply
}
