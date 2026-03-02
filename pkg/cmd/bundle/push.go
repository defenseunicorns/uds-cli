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

// PushOptions holds options for the push command.
type PushOptions struct {
	OCIReference string

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
		Use:   "push <bundle-oci-reference>",
		Short: "Push a bundle to an OCI registry",
		Long:  "Push a UDS bundle to an OCI registry using the provided OCI reference",
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
func (o *PushOptions) Complete(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.OCIReference = args[0]
	}
	return nil
}

// Validate validates the options.
func (o *PushOptions) Validate() error {
	if o.OCIReference == "" {
		return fmt.Errorf("OCI reference is required")
	}
	return nil
}

// Run executes the push command.
func (o *PushOptions) Run() error {
	slog.Debug("pushing bundle", "reference", o.OCIReference)
	if err := bundle.Push(o.OCIReference); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(o.Out, "✓ Bundle pushed successfully: %s\n", o.OCIReference)
	return nil
}
