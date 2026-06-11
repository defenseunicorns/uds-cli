// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
	"github.com/defenseunicorns/uds-cli/pkg/printer"
	"github.com/spf13/cobra"
)

// PullOptions holds options for the pull command.
type PullOptions struct {
	OCIReference string
	OutputDir    string
	Config       *bundle.UDSBundleConfig
	Printer      printer.ResourcePrinter

	iostreams.IOStreams
}

// NewPullOptions returns a PullOptions with default values.
func NewPullOptions(streams iostreams.IOStreams) *PullOptions {
	return &PullOptions{
		IOStreams: streams,
	}
}

// NewPullCommand creates the pull command.
func NewPullCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewPullOptions(streams)

	cmd := &cobra.Command{
		Use:   "pull <bundle-oci-reference>",
		Short: "Pull a bundle from an OCI registry",
		Long:  "Pull a UDS bundle from an OCI registry using the provided OCI reference",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			ctx := cmd.Context()
			util.CheckErr(o.Run(ctx))
		},
	}

	cmd.Flags().StringVarP(&o.OutputDir, "output-dir", "d", ".", "directory to write the pulled bundle tarball")

	return cmd
}

// Complete fills in options from command line args.
func (o *PullOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.OCIReference = args[0]
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	flags := SnapshotFlags(cmd)
	cfg, _, err := NewConfigResolver().Resolve(ctx, o.IOStreams, flags, "")
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

// Validate validates the options.
// Config validation is performed by the library entry point.
func (o *PullOptions) Validate() error {
	if o.OCIReference == "" {
		return fmt.Errorf("OCI reference is required")
	}
	if err := ValidateDir(o.OutputDir); err != nil {
		return fmt.Errorf("--output-dir: %w", err)
	}
	return nil
}

// Run executes the pull command.
func (o *PullOptions) Run(ctx context.Context) error {
	o.IOStreams = logger.Bind(o.IOStreams, o.Config.Global.LogLevel)
	o.Debug("pulling bundle", "ref", o.OCIReference, "output", o.OutputDir)
	if o.Config.Global.Prompt {
		confirmed, err := PromptConfirmation(o.IOStreams, "Pull this bundle?")
		if err != nil {
			return err
		}
		if !confirmed {
			o.Info("pull cancelled")
			return nil
		}
	}
	result, err := bundle.Pull(ctx, o.OCIReference, o.OutputDir, bundle.PullOptions{
		Config:  o.Config,
		Streams: o.IOStreams,
	})
	if err != nil {
		return err
	}
	return o.Printer.PrintObj(result, o.Out())
}
