// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"

	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

const bundleSignatureNotCheckedWarning = "bundle signature verification was not performed; package verification policy and signing metadata do not establish bundle integrity"

// InspectOptions holds the options for the inspect command.
type InspectOptions struct {
	BundlePath string // Path to a .tar.zst artifact or OCI reference
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
		Use:   "inspect <bundle-reference>",
		Short: "Inspect a UDS bundle",
		Long:  "Inspect a built UDS bundle from a local .tar.zst artifact or OCI reference, displaying metadata and package details.",
		Args:  cobra.ExactArgs(1),
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
		o.BundlePath = ""
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	flags := SnapshotFlags(cmd)
	// Use the embedded definition; skip sibling defaults.
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

// Validate validates the options without modifying state.
func (o *InspectOptions) Validate() error {
	if err := (bundle.InspectOptions{
		Source: o.BundlePath,
		Config: o.Config,
	}).Validate(); err != nil {
		return err
	}
	if bundle.IsOCIReference(o.BundlePath) {
		return nil
	}
	info, err := os.Stat(o.BundlePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("bundle artifact not found: %s", o.BundlePath)
		}
		return fmt.Errorf("cannot access bundle artifact %s: %w", o.BundlePath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("bundle artifact path is a directory: %s", o.BundlePath)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("bundle artifact path is not a regular file: %s", o.BundlePath)
	}
	return nil
}

// Run executes the inspect command.
func (o *InspectOptions) Run(ctx context.Context) error {
	o.IOStreams = logger.Bind(o.IOStreams, o.Config.Global.LogLevel)
	o.Debug("inspecting bundle", "source", o.BundlePath)

	result, err := bundle.Inspect(ctx, bundle.InspectOptions{
		Source:  o.BundlePath,
		Config:  o.Config,
		Streams: o.IOStreams,
	})
	if err != nil {
		return fmt.Errorf("inspecting bundle: %w", err)
	}
	if result.BundleSignature != nil && result.BundleSignature.Status == bundle.BundleSignatureStatusNotChecked {
		o.Warn(bundleSignatureNotCheckedWarning)
	}

	return o.Printer.PrintObj(result, o.Out())
}
