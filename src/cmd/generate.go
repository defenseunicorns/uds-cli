// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2021-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/generate"
	"github.com/spf13/cobra"
)

var generateCmd = &cobra.Command{
	Use:     "generate",
	Aliases: []string{"g"},
	Short:   lang.CmdGenerateShort,
	Long:    lang.CmdGenerateLong,
	Run: func(_ *cobra.Command, _ []string) {
		generate.Generate()
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.Flags().StringVarP(&config.GenerateChartUrl, "chart", "c", "", lang.CmdGenerateFlagChart)
	generateCmd.Flags().StringVarP(&config.GenerateChartName, "name", "n", "", lang.CmdGenerateFlagName)
	generateCmd.Flags().StringVarP(&config.GenerateChartVersion, "version", "v", "", lang.CmdGenerateFlagVersion)
	generateCmd.Flags().StringVarP(&config.GenerateOutputDir, "output", "o", "generated", lang.CmdGenerateOutputDir)
	generateCmd.MarkFlagRequired("chart")
	generateCmd.MarkFlagRequired("name")
	generateCmd.MarkFlagRequired("version")
}
