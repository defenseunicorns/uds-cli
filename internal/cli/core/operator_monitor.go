// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package core

import (
	"context"
	"fmt"
	"time"

	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	"github.com/defenseunicorns/uds-cli/internal/operator"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type monitorFunc func(context.Context, iostreams.IOStreams, operator.MonitorOptions) error

// OperatorMonitorOptions holds options for the core operator monitor command.
type OperatorMonitorOptions struct {
	iostreams.IOStreams

	Stream     operator.StreamKind
	Namespace  string
	Follow     bool
	Timestamps bool
	Since      time.Duration
	JSON       bool
	NoColor    bool
	LogLevel   string

	monitor monitorFunc
}

// NewOperatorMonitorOptions returns a new OperatorMonitorOptions with default values.
func NewOperatorMonitorOptions(streams iostreams.IOStreams) *OperatorMonitorOptions {
	return &OperatorMonitorOptions{IOStreams: streams, LogLevel: "info", monitor: operator.Monitor}
}

// NewOperatorMonitorCommand creates the core operator monitor command.
func NewOperatorMonitorCommand(streams iostreams.IOStreams) *cobra.Command {
	return newOperatorMonitorCommand(streams, operator.Monitor)
}

// newOperatorMonitorCommand is the underlying implementation of NewOperatorMonitorCommand.
// They are separate functions for the sake of testing, as being able to specify the monitorFunc allows tests
// to skip connecting to kubernetes
func newOperatorMonitorCommand(streams iostreams.IOStreams, monitor monitorFunc) *cobra.Command {
	o := NewOperatorMonitorOptions(streams)
	o.monitor = monitor

	cmd := &cobra.Command{
		Use:   "monitor [policies | operator | allowed | denied | failed | mutated]",
		Short: "Observe UDS Core operator activity",
		Long:  "Stream UDS Core operator activity and related Pepr policy decisions from the cluster.",
		Example: `  # Stream all operator and policy events
  uds core operator monitor

  # Follow UDS Core operator events
  uds core operator monitor operator --follow

  # Show policy denials from the last five minutes
  uds core operator monitor denied --since 5m`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run(cmd.Context()))
		},
	}

	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "only show events for this resource namespace")
	cmd.Flags().BoolVarP(&o.Follow, "follow", "f", false, "continuously stream logs")
	cmd.Flags().BoolVarP(&o.Timestamps, "timestamps", "t", false, "include timestamps")
	cmd.Flags().DurationVar(&o.Since, "since", 0, "only show logs newer than this duration")
	cmd.Flags().BoolVar(&o.JSON, "json", false, "output matching events as JSON")
	cmd.Flags().BoolVar(&o.NoColor, "no-color", false, "disable color output")

	return cmd
}

// Complete fills in options from command line args.
func (o *OperatorMonitorOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		o.Stream = operator.StreamKind(args[0])
	}
	if cmd.Root().PersistentFlags().Lookup("log-level") != nil {
		logLevel, err := cmd.Root().PersistentFlags().GetString("log-level")
		if err != nil {
			return fmt.Errorf("%w: %w", ErrReadLogLevel, err)
		}
		o.LogLevel = logLevel
	}
	return nil
}

// Validate validates the options.
func (o *OperatorMonitorOptions) Validate() error {
	switch o.Stream {
	case operator.StreamAll, operator.StreamPolicies, operator.StreamOperator, operator.StreamAllowed,
		operator.StreamDenied, operator.StreamFailed, operator.StreamMutated:
	default:
		return fmt.Errorf("%w: %s", ErrInvalidStreamKind, o.Stream)
	}
	if o.Since < 0 {
		return ErrInvalidSince
	}
	return nil
}

// Run executes operator monitoring.
func (o *OperatorMonitorOptions) Run(ctx context.Context) error {
	return o.monitor(ctx, o.IOStreams, operator.MonitorOptions{
		Stream:     o.Stream,
		Namespace:  o.Namespace,
		Follow:     o.Follow,
		Timestamps: o.Timestamps,
		Since:      o.Since,
		JSON:       o.JSON,
		NoColor:    o.NoColor,
		LogLevel:   o.LogLevel,
	})
}
