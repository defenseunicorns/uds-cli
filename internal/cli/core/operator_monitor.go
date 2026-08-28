// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package core

import (
	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	"github.com/defenseunicorns/uds-cli/internal/operator"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// OperatorMonitorOptions holds options for the core operator monitor command.
type OperatorMonitorOptions struct {
	iostreams.IOStreams

	NoColor bool
}

// NewOperatorMonitorOptions returns a new OperatorMonitorOptions with default values.
func NewOperatorMonitorOptions(streams iostreams.IOStreams) *OperatorMonitorOptions {
	return &OperatorMonitorOptions{
		IOStreams: streams,
	}
}

// NewOperatorMonitorCommand creates the core operator monitor command.
func NewOperatorMonitorCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewOperatorMonitorOptions(streams)

	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Observe UDS Core operator activity",
		Long:  "Observe UDS Core operator activity and related Pepr policy decisions",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	cmd.Flags().BoolVar(&o.NoColor, "no-color", false, "disable color output")

	return cmd
}

// Complete fills in options from command line args.
func (o *OperatorMonitorOptions) Complete(_ *cobra.Command, _ []string) error {
	return nil
}

// Validate validates the options.
func (o *OperatorMonitorOptions) Validate() error {
	return nil
}

// Run executes the operator monitor mockup.
func (o *OperatorMonitorOptions) Run() error {
	return operator.Monitor(o.Out(), operator.MonitorOptions{NoColor: o.NoColor})
}
