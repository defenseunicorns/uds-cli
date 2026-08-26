// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"github.com/defenseunicorns/uds-cli/internal/dev/disassemble"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/spf13/cobra"
)

func newDevDisassembleCommand() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "disassemble <source> <output-dir>",
		Short: "[beta] Convert a Zarf package into local source",
		Long:  "[beta] Pull a packaged artifact and rewrite it into a re-creatable local source directory. Zarf packages are supported today.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := printer.ParseFormat(output)
			if err != nil {
				return err
			}
			resultPrinter, err := printer.NewPrinter(format)
			if err != nil {
				return err
			}
			streams := iostreams.New(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			result, err := disassemble.Disassemble(cmd.Context(), disassemble.Options{
				Source:    args[0],
				OutputDir: args[1],
				Config: bundlepkg.ConfigOptions{
					Architecture:  config.CLIArch,
					PlainHTTP:     config.CommonOptions.Insecure,
					SkipTLSVerify: config.CommonOptions.Insecure,
					TmpDir:        config.CommonOptions.TempDirectory,
					Concurrency:   config.CommonOptions.OCIConcurrency,
				},
				Streams: streams,
			})
			if err != nil {
				return err
			}
			return resultPrinter.PrintObj(result, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format (text, json, yaml)")
	return cmd
}
