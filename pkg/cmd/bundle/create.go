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

// CreateOptions holds options for the create command.
type CreateOptions struct {
	BundlePath string // Path to bundle file or directory (user input, resolved in Run)
	Arch       string

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
			util.CheckErr(o.Run())
		},
	}

	cmd.Flags().StringVarP(&o.Arch, "arch", "a", "", "architecture to create bundle for (defaults to system architecture)")

	return cmd
}

// Complete fills in options from command line args.
func (o *CreateOptions) Complete(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundlePath = args[0]
	} else {
		o.BundlePath = "."
	}
	return nil
}

// Validate validates the options without modifying state.
func (o *CreateOptions) Validate() error {
	return ValidateBundlePath(o.BundlePath)
}

// Run executes the create command.
func (o *CreateOptions) Run() error {
	ctx := context.Background()

	// Resolve the bundle path
	bundlePath := ResolveBundlePath(o.BundlePath)

	_, _ = fmt.Fprintf(o.Out, "Creating bundle from file: %s\n", bundlePath)
	outPath, err := bundle.Create(ctx, bundle.CreateOptions{
		BundleFile: bundlePath,
		Arch:       o.Arch,
		Out:        o.Out,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(o.Out, "Bundle created: %s\n", outPath)
	return nil
}
