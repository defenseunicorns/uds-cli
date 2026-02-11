// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package version

import (
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/version"
	"github.com/spf13/cobra"
)

// Options holds options for the version command.
type Options struct {
	iostreams.IOStreams
}

// NewOptions returns a new Options with default values.
func NewOptions(streams iostreams.IOStreams) *Options {
	return &Options{
		IOStreams: streams,
	}
}

// NewVersionCommand creates the version command.
func NewVersionCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewOptions(streams)

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Long:  "Print the version, git commit, and build date of the UDS CLI",
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	return cmd
}

// Complete fills in options from command line args.
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	// No args or flags to process for version command
	return nil
}

// Validate validates the options.
func (o *Options) Validate() error {
	// Nothing to validate for version command
	return nil
}

// Run executes the version command.
func (o *Options) Run() error {
	fmt.Fprintf(o.Out, "uds version %s\n", version.Version)
	fmt.Fprintf(o.Out, "Git commit: %s\n", version.GitCommit)
	fmt.Fprintf(o.Out, "Build date: %s\n", version.BuildDate)
	return nil
}
