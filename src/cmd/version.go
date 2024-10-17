// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

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
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		config.SkipLogFile = true
		err := cliSetup(cmd)
		if err != nil {
			return err
		}
		return nil
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
