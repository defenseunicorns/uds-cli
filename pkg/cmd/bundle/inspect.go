// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// InspectOptions holds the options for the inspect command.
type InspectOptions struct {
	BundlePath string // Path to bundle file or directory (user input, resolved in Run)

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
func (o *InspectOptions) Complete(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundlePath = args[0]
	} else {
		// Default to looking for bundle.uds.hcl in current directory
		o.BundlePath = "."
	}
	return nil
}

// Validate validates the options without modifying state.
func (o *InspectOptions) Validate() error {
	return ValidateBundlePath(o.BundlePath)
}

// Run executes the inspect command.
func (o *InspectOptions) Run() error {
	// Resolve the bundle path
	bundlePath := ResolveBundlePath(o.BundlePath)

	b, err := bundle.NewHCLParser().ParseBundleFile(context.Background(), bundlePath)
	if err != nil {
		return err
	}

	if err := b.Validate(); err != nil {
		return fmt.Errorf("invalid bundle: %w", err)
	}

	out, err := b.BufferString()
	if err != nil {
		return err
	}
	_, err = o.Out.Write(out.Bytes())
	return err
}
