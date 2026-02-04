// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// CreateOptions holds options for the create command.
type CreateOptions struct {
	BundleFile string

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
		Use:   "create <bundle-file>",
		Short: "Create a new UDS bundle",
		Long:  "Create a new UDS bundle from an HCL configuration file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	return cmd
}

// Complete fills in options from command line args.
func (o *CreateOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundleFile = args[0]
	}
	return nil
}

// Validate validates the options.
func (o *CreateOptions) Validate() error {
	if o.BundleFile == "" {
		return fmt.Errorf("bundle file is required")
	}
	return nil
}

// Run executes the create command.
func (o *CreateOptions) Run() error {
	fmt.Fprintf(o.Out, "Creating bundle from file: %s\n", o.BundleFile)
	return bundle.Create(o.BundleFile)
}
