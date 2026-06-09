// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
	"github.com/defenseunicorns/uds-cli/pkg/printer"
	"github.com/spf13/cobra"
)

// PushOptions holds options for the push command.
type PushOptions struct {
	Tarball      string
	OCIReference string
	Config       *bundle.UDSBundleConfig
	Printer      printer.ResourcePrinter

	iostreams.IOStreams
}

// NewPushOptions returns a PushOptions with default values.
func NewPushOptions(streams iostreams.IOStreams) *PushOptions {
	return &PushOptions{
		IOStreams: streams,
	}
}

// NewPushCommand creates the push command.
func NewPushCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewPushOptions(streams)

	cmd := &cobra.Command{
		Use:   "push <bundle-tarball> <oci-reference>",
		Short: "Push a bundle to an OCI registry",
		Long:  "Push a UDS bundle tarball to a remote OCI registry",
		Args:  cobra.ExactArgs(2),
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
func (o *PushOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.Tarball = args[0]
	}
	if len(args) > 1 {
		o.OCIReference = args[1]
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
func (o *PushOptions) Validate() error {
	if o.Tarball == "" || !strings.HasSuffix(o.Tarball, ".tar.zst") {
		return fmt.Errorf("source must be a .tar.zst bundle file")
	}
	if _, err := os.Stat(o.Tarball); os.IsNotExist(err) {
		return fmt.Errorf("bundle file not found: %s", o.Tarball)
	}
	if o.OCIReference == "" {
		return fmt.Errorf("OCI reference is required")
	}
	return nil
}

// Run executes the push command.
func (o *PushOptions) Run(ctx context.Context) error {
	o.IOStreams = logger.Bind(o.IOStreams, o.Config.Global.LogLevel)
	o.Debug("pushing bundle", "tarball", o.Tarball, "ref", o.OCIReference)
	result, err := bundle.Push(ctx, o.Tarball, o.OCIReference, bundle.PushOptions{
		Config:  o.Config,
		Streams: o.IOStreams,
	})
	if err != nil {
		return err
	}
	return o.Printer.PrintObj(result, o.Out())
}
