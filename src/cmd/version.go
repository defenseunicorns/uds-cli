// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2021-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"fmt"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Aliases: []string{"v"},
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		config.SkipLogFile = true
		cliSetup(cmd)
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
