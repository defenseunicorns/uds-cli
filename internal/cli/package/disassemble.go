// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packagecli

import (
	"context"

	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/mode/disassemble"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// DisassembleOptions holds options for package disassembly.
type DisassembleOptions struct {
	disassemble.Options
	LogLevelName string

	iostreams.IOStreams
}

type disassembleResult struct {
	Source    string `json:"source" yaml:"source" text:"Source"`
	OutputDir string `json:"outputDir" yaml:"outputDir" text:"Output Directory"`
}

// NewDisassembleOptions returns package disassembly options.
func NewDisassembleOptions(streams iostreams.IOStreams) *DisassembleOptions {
	return &DisassembleOptions{IOStreams: streams}
}

// NewDisassembleCommand creates the package disassemble command.
func NewDisassembleCommand(streams iostreams.IOStreams) *cobra.Command {
	return newDisassembleCommand(NewDisassembleOptions(streams))
}

func newDisassembleCommand(o *DisassembleOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "disassemble <source> <output-dir>",
		Short: "Convert a Zarf package into rebuildable offline source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Complete(cmd, args); err != nil {
				return err
			}
			return o.Run(cmd.Context())
		},
	}
}

// Complete fills options from command-line arguments and flags.
func (o *DisassembleOptions) Complete(cmd *cobra.Command, args []string) error {
	o.Source = args[0]
	o.OutputDir = args[1]
	var err error
	o.LogLevelName, err = completePackageOptions(cmd, &o.PackageOptions)
	return err
}

// Run disassembles the source package and prints its result as text.
func (o *DisassembleOptions) Run(ctx context.Context) error {
	o.IOStreams = logger.Bind(o.IOStreams, o.LogLevelName)
	o.Options.Warn = o.IOStreams.Warn
	outputDir, err := disassemble.Disassemble(ctx, o.Options)
	if err != nil {
		return err
	}
	result := &disassembleResult{Source: o.Source, OutputDir: outputDir}
	return (&printer.TextPrinter{}).PrintObj(result, o.Out())
}
