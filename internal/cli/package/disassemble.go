// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packagecli

import (
	"context"
	"errors"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/mode/disassemble"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// DisassembleOptions holds options for package disassembly.
type DisassembleOptions struct {
	Source    string
	OutputDir string
	commonOptions

	iostreams.IOStreams
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
			if err := o.Validate(); err != nil {
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
	if err := completeCommonOptions(cmd, &o.commonOptions); err != nil {
		return err
	}
	return nil
}

// Validate validates package disassembly options.
func (o *DisassembleOptions) Validate() error {
	switch {
	case strings.TrimSpace(o.Source) == "":
		return errors.New("source is required")
	case strings.TrimSpace(o.OutputDir) == "":
		return errors.New("output directory is required")
	case strings.TrimSpace(o.TmpDir) == "":
		return errors.New("temporary directory is required")
	case o.Concurrency < 1:
		return errors.New("concurrency must be greater than zero")
	default:
		return nil
	}
}

// Run disassembles the source package and prints its result as text.
func (o *DisassembleOptions) Run(ctx context.Context) error {
	o.IOStreams = logger.Bind(o.IOStreams, o.LogLevelName)
	zarf.ConfigureTempDir(o.TmpDir)
	result, err := disassemble.Disassemble(ctx, disassemble.Options{
		Source:               o.Source,
		OutputDir:            o.OutputDir,
		PlainHTTP:            o.PlainHTTP,
		SkipTLSVerify:        o.SkipTLSVerify,
		TmpDir:               o.TmpDir,
		CachePath:            o.CachePath,
		Concurrency:          o.Concurrency,
		VerificationStrategy: layout.VerifyIfPossible,
		Warn:                 o.Warn,
	})
	if err != nil {
		return err
	}
	return (&printer.TextPrinter{}).PrintObj(result, o.Out())
}
