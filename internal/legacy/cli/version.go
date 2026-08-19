// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config/lang"
	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Aliases: []string{"v"},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Root().PersistentFlags().Set("no-log-file", "true"); err != nil {
				return err
			}
			return legacyPreRun(cmd)
		},
		Short: lang.CmdVersionShort,
		Long:  lang.CmdVersionLong,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(config.CLIVersion)
		},
	}
}
