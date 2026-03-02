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

// PullOptions holds options for the pull command.
type PullOptions struct {
	OCIReference string

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
			util.CheckErr(o.Run())
		},
	}

	return cmd
}

// Complete fills in options from command line args.
func (o *PullOptions) Complete(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.OCIReference = args[0]
	}
	return nil
}

// Validate validates the options.
func (o *PullOptions) Validate() error {
	if o.OCIReference == "" {
		return fmt.Errorf("OCI reference is required")
	}
	return nil
}

// Run executes the pull command.
func (o *PullOptions) Run() error {
	slog.Debug("pulling bundle", "reference", o.OCIReference)
	if err := bundle.Pull(o.OCIReference); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(o.Out, "✓ Bundle pulled successfully: %s\n", o.OCIReference)
	return nil
}
