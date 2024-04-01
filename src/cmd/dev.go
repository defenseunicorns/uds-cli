// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/zarf/src/pkg/message"

	"github.com/defenseunicorns/uds-cli/src/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle/tui/deploy"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: lang.CmdDevShort,
}

var devDeployCmd = &cobra.Command{
	Use:   "deploy",
	Args:  cobra.MaximumNArgs(1),
	Short: lang.CmdDevDeployShort,
	PreRun: func(_ *cobra.Command, args []string) {
		setBundleFile(args)
	},
	Run: func(_ *cobra.Command, args []string) {

		// (TODO: remove once we have create tui code)create an empty program and kill it, this makes Program.Send a no-op
		deploy.Program = tea.NewProgram(nil)
		deploy.Program.Kill()

		// Create Bundle
		srcDir, err := os.Getwd()
		if err != nil {
			message.Fatalf(err, "error reading the current working directory")
		}
		if len(args) > 0 {
			srcDir = args[0]
		}

		if len(srcDir) != 0 && srcDir[len(srcDir)-1] != '/' {
			srcDir = srcDir + "/"
		}

		config.CommonOptions.Confirm = true
		bundleCfg.CreateOpts.SourceDirectory = srcDir
		configureZarf()

		// load uds-config if it exists
		if v.ConfigFileUsed() != "" {
			if err := loadViperConfig(); err != nil {
				message.Fatalf(err, "Failed to load uds-config: %s", err.Error())
				return
			}
		}

		bndlClient := bundle.NewOrDie(&bundleCfg)
		defer bndlClient.ClearPaths()

		// Check if local zarf packages need to be created
		bndlClient.CreateZarfPkgs()

		// Create dev bundle
		config.Dev = true
		if err := bndlClient.Create(); err != nil {
			bndlClient.ClearPaths()
			message.Fatalf(err, "Failed to create bundle: %s", err.Error())
		}

		// Deploy dev bundle
		bndlClient.SetDevSource(srcDir)

		// don't use bubbletea if --no-tea flag is set
		if config.CommonOptions.NoTea {
			deployWithoutTea(bndlClient)
			return
		}

		// start up bubbletea
		m := deploy.InitModel(bndlClient)

		// detect tty so CI/containers don't break
		if term.IsTerminal(int(os.Stdout.Fd())) {
			deploy.Program = tea.NewProgram(&m)
		} else {
			deploy.Program = tea.NewProgram(&m, tea.WithInput(nil))
		}

		if _, err := deploy.Program.Run(); err != nil {
			message.Fatalf(err, "TUI program error: %s", err.Error())
		}
	},
}

func init() {
	initViper()
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devDeployCmd)
}
