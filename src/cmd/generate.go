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

var generateCmd = &cobra.Command{
	Use:     "generate --chart [HELM CHART URL] --version [HELM CHART VERSION]",
	Aliases: []string{"g"},
	Short:   lang.CmdGenerateShort,
	Long:    lang.CmdGenerateLong,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Generating some things")
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.Flags().StringVarP(&config.GenerateChartUrl, "chart", "c", "", lang.CmdGenerateFlagChart)
	generateCmd.Flags().StringVarP(&config.GenerateChartVersion, "version", "v", "", lang.CmdGenerateFlagVersion)
}
