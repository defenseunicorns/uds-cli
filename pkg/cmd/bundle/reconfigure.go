// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/printer"
	"github.com/spf13/cobra"
)

// ReconfigureOptions holds options for the reconfigure command.
type ReconfigureOptions struct {
	Source       string
	DefaultsFile string
	Suffix       string
	OutputDir    string
	Options      bundle.ConfigOptions
	Printer      printer.ResourcePrinter

	iostreams.IOStreams
}

// NewReconfigureOptions returns a ReconfigureOptions with default values.
func NewReconfigureOptions(streams iostreams.IOStreams) *ReconfigureOptions {
	return &ReconfigureOptions{
		Suffix:   "-reconfigured",
		IOStreams: streams,
	}
}

// NewReconfigureCommand creates the reconfigure command.
func NewReconfigureCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewReconfigureOptions(streams)

	cmd := &cobra.Command{
		Use:   "reconfigure <source> --defaults <defaults-file>",
		Short: "Reconfigure a bundle with new default values",
		Long:  "Replace the defaults in a bundle artifact with new values, producing a new derivative artifact",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	cmd.Flags().String("defaults", "", "path to new defaults.uds.hcl file (required)")
	_ = cmd.MarkFlagRequired("defaults")
	cmd.Flags().String("suffix", "-reconfigured", "suffix for output artifact name")
	cmd.Flags().StringVarP(&o.OutputDir, "output-dir", "", "", "output directory for reconfigured local tarball (default: current directory)")

	return cmd
}

// Complete fills in options from command args and flags.
func (o *ReconfigureOptions) Complete(cmd *cobra.Command, args []string) error {
	o.Source = args[0]

	var err error
	o.DefaultsFile, err = cmd.Flags().GetString("defaults")
	if err != nil {
		return err
	}

	o.Suffix, err = cmd.Flags().GetString("suffix")
	if err != nil {
		return err
	}

	// Default OutputDir to CWD for local sources.
	if o.OutputDir == "" && !bundle.IsOCIReference(o.Source) {
		o.OutputDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
	}

	// Resolve config options from persistent flags (plain-http, skip-tls-verify, etc.)
	cfg, _, err := NewConfigResolver().Resolve(cmd, "")
	if err != nil {
		return err
	}
	if cfg.Options != nil {
		o.Options = *cfg.Options
	}

	p, err := ResolvePrinter(cmd)
	if err != nil {
		return err
	}
	o.Printer = p

	return nil
}

// Validate checks that options are valid.
func (o *ReconfigureOptions) Validate() error {
	if o.Source == "" {
		return fmt.Errorf("source is required")
	}
	if o.DefaultsFile == "" {
		return fmt.Errorf("--defaults is required")
	}
	if _, err := os.Stat(o.DefaultsFile); err != nil {
		return fmt.Errorf("defaults file not found: %s", o.DefaultsFile)
	}
	if o.Suffix == "" {
		return fmt.Errorf("--suffix must not be empty")
	}

	if o.OutputDir != "" && bundle.IsOCIReference(o.Source) {
		return fmt.Errorf("--output-dir is not supported for OCI sources")
	}

	return nil
}

// Run executes the reconfigure command.
func (o *ReconfigureOptions) Run() error {
	result, err := bundle.Reconfigure(context.Background(), bundle.ReconfigureOptions{
		Source:       o.Source,
		DefaultsFile: o.DefaultsFile,
		Suffix:       o.Suffix,
		OutputDir:    o.OutputDir,
		Options:      o.Options,
	})
	if err != nil {
		return err
	}

	return o.Printer.PrintObj(result, o.Out)
}
