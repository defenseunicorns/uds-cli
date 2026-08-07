// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
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
			ctx := cmd.Context()
			util.CheckErr(o.Run(ctx))
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

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	flags := SnapshotFlags(cmd)
	cfg, _, err := NewConfigResolver().Resolve(ctx, o.IOStreams, flags, o.BundlePath)
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
	return ValidateBundlePath(o.BundlePath)
}

// Run executes the inspect command.
func (o *InspectOptions) Run(ctx context.Context) error {
	// Resolve the bundle path
	bundlePath := bundle.ResolveBundlePath(o.BundlePath)
	o.IOStreams = logger.Bind(o.IOStreams, o.Config.Global.LogLevel)
	o.Debug("inspecting bundle", "path", bundlePath)

	// This method diverges from the 0004-logging-and-output-strategy.md recommendation and does not have a clear
	// business layer method. Perhaps we'll introduce it in the future once there are more logic there than just
	// bundle parsing.
	b, err := bundle.NewHCLParser(o.Config.Options.Architecture, o.IOStreams).ParseBundleFile(ctx, bundlePath)
	if err != nil {
		return err
	}

	if err := b.Validate(); err != nil {
		return fmt.Errorf("invalid bundle: %w", err)
	}

	result, err := bundle.ToInspectResult(ctx, b, o.IOStreams)
	if err != nil {
		return fmt.Errorf("inspecting bundle: %w", err)
	}

	return o.Printer.PrintObj(result, o.Out())
}
