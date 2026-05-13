// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/printer"
	"github.com/spf13/cobra"
)

// InspectOptions holds the options for the inspect command.
type InspectOptions struct {
	BundlePath string // Path to bundle file or directory (user input, resolved in Run)
	Config     *bundle.UDSBundleConfig
	Printer    printer.ResourcePrinter

	iostreams.IOStreams
}

// NewInspectOptions returns a new InspectOptions with default values.
func NewInspectOptions(streams iostreams.IOStreams) *InspectOptions {
	return &InspectOptions{
		IOStreams: streams,
	}
}

// NewInspectCommand creates the inspect command.
func NewInspectCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewInspectOptions(streams)

	cmd := &cobra.Command{
		Use:   "inspect [bundle-path]",
		Short: "Inspect a UDS bundle",
		Long:  "Inspect a UDS bundle definition from a local HCL file or directory, displaying metadata and package details.",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	return cmd
}

// Complete fills in options from command line args.
func (o *InspectOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundlePath = args[0]
	} else {
		// Default to looking for bundle.uds.hcl in current directory
		o.BundlePath = "."
	}

	cfg, _, err := NewConfigResolver().Resolve(cmd, o.BundlePath)
	if err != nil {
		return err
	}
	o.Config = cfg

	p, err := ResolvePrinter(cmd)
	if err != nil {
		return err
	}
	o.Printer = p

	return nil
}

// Validate validates the options without modifying state.
func (o *InspectOptions) Validate() error {
	if err := bundle.ValidateConfig(o.Config); err != nil {
		return err
	}
	return ValidateBundlePath(o.BundlePath)
}

// Run executes the inspect command.
func (o *InspectOptions) Run() error {
	// Resolve the bundle path
	bundlePath := ResolveBundlePath(o.BundlePath)
	slog.Debug("inspecting bundle", "path", bundlePath)

	// This method diverges from the 0004-logging-and-output-strategy.md recommendation and does not have a clear
	// business layer method. Perhaps we'll introduce it in the future once there are more logic there than just
	// bundle parsing.
	b, err := bundle.NewHCLParser(o.Config.Options.Architecture).ParseBundleFile(context.Background(), bundlePath)
	if err != nil {
		return err
	}

	if err := b.Validate(); err != nil {
		return fmt.Errorf("invalid bundle: %w", err)
	}

	result, err := b.ToInspectResult()
	if err != nil {
		return fmt.Errorf("inspecting bundle: %w", err)
	}

	return o.Printer.PrintObj(result, o.Out)
}
