// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/dev/disassemble"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// DevDisassembleOptions holds options for artifact disassembly.
type DevDisassembleOptions struct {
	Source    string
	OutputDir string
	Config    *bundlepkg.UDSBundleConfig
	Printer   printer.ResourcePrinter

	run disassembleRunner
	iostreams.IOStreams
}

type disassembleRunner func(context.Context, disassemble.Options) (*disassemble.Result, error)

// NewDevDisassembleOptions returns development disassembly options.
func NewDevDisassembleOptions(streams iostreams.IOStreams) *DevDisassembleOptions {
	return &DevDisassembleOptions{IOStreams: streams, run: disassemble.Disassemble}
}

// NewDevDisassembleCommand creates the bundle development disassemble command.
func NewDevDisassembleCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewDevDisassembleOptions(streams)
	return &cobra.Command{
		Use:   "disassemble <source> <output-dir>",
		Short: "Convert a Zarf package into local source",
		Long:  "Pull a packaged artifact and rewrite it into a re-creatable local source directory. Zarf packages are supported today.",
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

// Complete fills options from command-line arguments and inherited flags.
func (o *DevDisassembleOptions) Complete(cmd *cobra.Command, args []string) error {
	o.Source = args[0]
	o.OutputDir = args[1]
	cfg, _, err := NewConfigResolver().Resolve(cmd.Context(), o.IOStreams, SnapshotFlags(cmd), "")
	if err != nil {
		return err
	}
	o.Config = cfg
	o.Printer, err = ResolvePrinter(cmd)
	return err
}

// Validate validates artifact disassembly options.
func (o *DevDisassembleOptions) Validate() error {
	if strings.TrimSpace(o.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if strings.TrimSpace(o.OutputDir) == "" {
		return fmt.Errorf("output directory is required")
	}
	if o.Config == nil || o.Config.Options == nil {
		return fmt.Errorf("configuration is required")
	}
	return nil
}

// Run disassembles the source artifact and prints its result.
func (o *DevDisassembleOptions) Run(ctx context.Context) error {
	result, err := o.run(ctx, disassemble.Options{
		Source:    o.Source,
		OutputDir: o.OutputDir,
		Config:    *o.Config.Options,
		Streams:   o.IOStreams,
	})
	if err != nil {
		return err
	}
	return o.Printer.PrintObj(result, o.Out())
}
