// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
	"github.com/defenseunicorns/uds-cli/pkg/printer"
	"github.com/spf13/cobra"
)

// CreateOptions holds options for the create command.
type CreateOptions struct {
	BundlePath string // Path to bundle file or directory (user input, resolved in Run)
	Config     *bundle.UDSBundleConfig
	Printer    printer.ResourcePrinter

	iostreams.IOStreams
}

// NewCreateOptions returns a CreateOptions with default values.
func NewCreateOptions(streams iostreams.IOStreams) *CreateOptions {
	return &CreateOptions{
		IOStreams: streams,
	}
}

// NewCreateCommand creates the create command.
func NewCreateCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewCreateOptions(streams)

	cmd := &cobra.Command{
		Use:   "create [directory]",
		Short: "Create a new UDS bundle",
		Long:  "Create a new UDS bundle from an HCL configuration file",
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
func (o *CreateOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundlePath = args[0]
	} else {
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
// Config validation is performed by the library entry point.
func (o *CreateOptions) Validate() error {
	return ValidateBundlePath(o.BundlePath)
}

// Run executes the create command.
func (o *CreateOptions) Run(ctx context.Context) error {
	// Resolve the bundle path
	bundlePath := bundle.ResolveBundlePath(o.BundlePath)
	o.IOStreams = logger.Bind(o.IOStreams, o.Config.Global.LogLevel)
	o.Debug("creating bundle", "path", bundlePath)

	result, err := bundle.Create(ctx, bundle.CreateOptions{
		Config:     o.Config,
		BundleFile: bundlePath,
		Streams:    o.IOStreams,
	})
	if err != nil {
		return err
	}

	return o.Printer.PrintObj(result, o.Out())
}
