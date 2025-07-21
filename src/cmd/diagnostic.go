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

var optionUploadArchive bool = false

var diagnosticCollectCmd = &cobra.Command{
	Use:   "collect",
	Args:  cobra.MaximumNArgs(0),
	Short: lang.CmdDiagnosticCollectShort,
	Long:  lang.CmdDiagnosticCollectShort,
	RunE: func(cmd *cobra.Command, args []string) error {
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
		collectors = append(collectors, &diagnostic.SecretCollector{})

		anonymizer, _ := diagnostic.NewBuilder().Build()

		filter := &diagnostic.AcceptAllFilter{}

		var collectionResults []diagnostic.CollectionResult

		collectionResults = append(collectionResults, diagnostic.Collect(cmd.Context(), "", filter, collectors, anonymizer))

		fmt.Printf("\n\n==== Collected Data ====\n\n")

		storeDirectory, err := diagnostic.DiagnosticDirectory()
		if err != nil {
			fmt.Printf("failed to obtain directory for collecting the results: %v", err)
			return err
		}

		if len(collectionResults) > 0 {
			directoryName, compressedFileName, err := diagnostic.WriteToFile(storeDirectory, collectionResults...)
			if err != nil {
				return err
			}

			fmt.Printf("Collected %d files\n", len(collectionResults[0].RawObjects))
			fmt.Printf("Anonymized %d objects\n", anonymizer.AnonymizedEntries())
			if (len(collectionResults[0].Errors)) > 0 {
				for _, err := range collectionResults[0].Errors {
					fmt.Printf("Collection error %s\n", err)
				}
			}

			fmt.Printf("Diagnostic data file (compressed): %v\n", compressedFileName)
			fmt.Printf("Diagnostic data directory: %v\n", directoryName)

			uploader := diagnostic.S3Uploader{
				BucketName: "sebastian-2025-dash-days",
				Region:     "us-gov-west-1",
			}

			if optionUploadArchive {
				err = uploader.UploadFile(cmd.Context(), compressedFileName)
				if err != nil {
					fmt.Printf("failed to upload file to S3: %v", err)
					return err
				}

				fmt.Printf("Diagnostic data uploaded to sebastian-2025-dash-days bucket\n")
			}
		}

		return nil
	},
}

func init() {
	initViper()
	rootCmd.AddCommand(diagnosticCmd)
	diagnosticCmd.AddCommand(diagnosticCollectCmd)
	diagnosticCollectCmd.Flags().BoolVar(&optionUploadArchive, "upload", false, lang.CmdDiagnosticUploadShort)
}
