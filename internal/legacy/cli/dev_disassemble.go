// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/mode/disassemble"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/message"
	"github.com/spf13/cobra"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"gopkg.in/yaml.v3"
)

func newDevDisassembleCommand() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "disassemble <source> <output-dir>",
		Short: "[beta] Convert a Zarf package into rebuildable offline source",
		Long:  "[beta] Extract a complete Zarf package and rewrite its packaged resources into a local source directory that can be rebuilt offline. Zarf packages are supported today.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := disassemble.Disassemble(cmd.Context(), legacyDisassembleOptions(args[0], args[1]))
			if err != nil {
				return err
			}
			return printDisassembleResult(cmd, strings.ToLower(output), result)
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format (text, json, yaml)")
	return cmd
}

func legacyDisassembleOptions(source, outputDir string) disassemble.Options {
	return disassemble.Options{
		Source:               source,
		OutputDir:            outputDir,
		Architecture:         config.CLIArch,
		PlainHTTP:            config.CommonOptions.Insecure,
		SkipTLSVerify:        config.CommonOptions.Insecure,
		TmpDir:               config.CommonOptions.TempDirectory,
		Concurrency:          config.CommonOptions.OCIConcurrency,
		VerificationStrategy: legacyDisassembleVerificationStrategy(),
		Warn: func(msg string, args ...any) {
			message.Warnf("%s: %v", msg, args)
		},
	}
}

func legacyDisassembleVerificationStrategy() layout.VerificationStrategy {
	if config.CommonOptions.SkipSignatureValidation {
		return layout.VerifyNever
	}
	return layout.VerifyIfPossible
}

func printDisassembleResult(cmd *cobra.Command, format string, result *disassemble.Result) error {
	switch format {
	case "", "text":
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Source:            %s\nOutput Directory:  %s\n", result.Source, result.OutputDir)
		return err
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case "yaml":
		return yaml.NewEncoder(cmd.OutOrStdout()).Encode(result)
	default:
		return fmt.Errorf("unknown output format %q, valid values are: text, json, yaml", format)
	}
}
