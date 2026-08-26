// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2021-Present the Maru Authors

// Package cmd contains the CLI commands for maru.
package cmd

import (
	"fmt"

	"github.com/defenseunicorns/maru-runner/src/config"
	"github.com/defenseunicorns/maru-runner/src/config/lang"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Aliases: []string{"v"},
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		skipLogFile = true
		cliSetup()
	},
	Short: lang.CmdVersionShort,
	Long:  lang.CmdVersionLong,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println(config.CLIVersion)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
