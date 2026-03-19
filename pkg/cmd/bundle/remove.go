// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
	"log/slog"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// RemoveOptions holds options for the remove command.
type RemoveOptions struct {
	OCIReference string
	Config       *bundle.UDSBundleConfig

	iostreams.IOStreams
}

// NewRemoveOptions returns a RemoveOptions with default values.
func NewRemoveOptions(streams iostreams.IOStreams) *RemoveOptions {
	return &RemoveOptions{
		IOStreams: streams,
	}
}

// NewRemoveCommand creates the remove command.
func NewRemoveCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewRemoveOptions(streams)

	cmd := &cobra.Command{
		Use:   "remove <bundle-oci-reference>",
		Short: "Remove a bundle from a Kubernetes cluster",
		Long:  "Remove a UDS bundle from a Kubernetes cluster using the provided OCI reference",
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
func (o *RemoveOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.OCIReference = args[0]
	}

	cfg, _, err := NewConfigResolver().Resolve(cmd)
	if err != nil {
		return err
	}
	o.Config = cfg
	return nil
}

// Validate validates the options.
func (o *RemoveOptions) Validate() error {
	if o.OCIReference == "" {
		return fmt.Errorf("OCI reference is required")
	}
	return nil
}

// Run executes the remove command.
func (o *RemoveOptions) Run() error {
	slog.Debug("removing bundle", "reference", o.OCIReference)
	if err := bundle.Remove(o.OCIReference); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(o.Out, "✓ Bundle removed successfully: %s\n", o.OCIReference)
	return nil
}
