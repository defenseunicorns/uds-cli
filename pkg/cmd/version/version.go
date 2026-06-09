// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package version provides the version command for the UDS CLI.
package version

import (
	"bytes"
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
func (o *Options) Complete(_ *cobra.Command, _ []string) error {
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
	var out bytes.Buffer
	if _, err := fmt.Fprintf(&out, "uds version %s\n", version.Version); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(&out, "Git commit: %s\n", version.GitCommit); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(&out, "Build date: %s\n", version.BuildDate); err != nil {
		return err
	}
	_, err := o.Out().Write(out.Bytes())
	return err
}
