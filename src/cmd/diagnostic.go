// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"fmt"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/diagnostic"
	"github.com/spf13/cobra"
)

var diagnosticCmd = &cobra.Command{
	Use:   "diagnostic",
	Short: lang.CmdDiagnosticShort,
}

var diagnosticCollectCmd = &cobra.Command{
	Use:   "collect",
	Args:  cobra.MaximumNArgs(0),
	Short: lang.CmdDiagnosticCollectShort,
	Long:  lang.CmdDiagnosticCollectShort,
	RunE: func(cmd *cobra.Command, args []string) error {
		//ctx := cmd.Context()

		fmt.Println("Collecting diagnostic information...\n")

		var collectors []diagnostic.Collector
		collectors = append(collectors, &diagnostic.ScriptCollector{
			ScriptName: "statefulsets",
		})
		collectors = append(collectors, &diagnostic.ScriptCollector{
			ScriptName: "packages",
		})
		collectors = append(collectors, &diagnostic.ScriptCollector{
			ScriptName: "overview",
		})
		collectors = append(collectors, &diagnostic.LogsCollector{})

		anonymizer := &diagnostic.SensitiveDataAnonymizer{}

		filter := &diagnostic.AcceptAllFilter{}

		var collectionResults []diagnostic.CollectionResult

		collectionResults = append(collectionResults, diagnostic.Collect(cmd.Context(), "", filter, collectors, anonymizer))

		fmt.Printf("\n\n==== Collected Data ====\n\n")

		storeDirectory, err := diagnostic.DebugDirectory()
		if err != nil {
			fmt.Printf("failed to obtain directory for collecting the results: %v", err)
			return err
		}

		if len(collectionResults) > 0 {
			directoryName, compressedFileName, err := diagnostic.WriteToFile(storeDirectory, collectionResults...)
			if err != nil {
				return err
			}
			fmt.Printf("Debug data file (compressed): %v\n", compressedFileName)
			fmt.Printf("Debug data directory: %v\n", directoryName)
		}

		return nil
	},
}

func init() {
	initViper()
	rootCmd.AddCommand(diagnosticCmd)
	diagnosticCmd.AddCommand(diagnosticCollectCmd)
}
