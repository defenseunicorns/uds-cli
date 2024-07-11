// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package monitor contains the CLI commands for UDS monitor.
package monitor

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/engine/pepr"
	"github.com/defenseunicorns/uds-cli/src/pkg/engine/stream"
	"github.com/spf13/cobra"
)

var follow bool
var timestamps bool
var since time.Duration
var json bool

var peprCmd = &cobra.Command{
	Use:     "pepr [policies | operator | allowed | denied | failed | mutated]",
	Aliases: []string{"p"},
	Example: `
  # Aggregates all admission and operator logs into a single stream
  uds monitor pepr

  # Stream UDS Operator actions (Package processing, status updates, and errors)
  uds monitor pepr operator

  # Stream UDS Policy logs (Allow, Deny, Mutate)
  uds monitor pepr policies

  # Stream UDS Policy allow logs
  uds monitor pepr allowed

  # Stream UDS Policy deny logs
  uds monitor pepr denied

  # Stream UDS Policy mutation logs
  uds monitor pepr mutated

  # Stream UDS Policy deny logs and UDS Operator error logs
  uds monitor pepr failed`,
	Short: lang.CmdMonitorPeprShort,
	Long:  lang.CmdMonitorPeprLong,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		// Set the stream kind from the CLI
		var streamKind pepr.StreamKind
		if len(args) > 0 && args[0] != "" {
			streamKind = pepr.StreamKind(args[0])

			// Validate the stream kind
			switch streamKind {
			case pepr.PolicyStream, pepr.OperatorStream, pepr.AllowStream, pepr.DenyStream, pepr.FailureStream, pepr.MutateStream:
				// Valid stream kind
			default:
				return fmt.Errorf("Invalid stream kind: %s", string(streamKind))
			}
		}

		// Create a new stream for the Pepr logs
		peprReader := pepr.NewStreamReader(namespace, "")
		peprStream := stream.NewStream(os.Stdout, peprReader, "pepr-system")

		// Set the stream flags
		peprReader.JSON = json
		peprReader.FilterStream = streamKind
		peprStream.Follow = follow
		peprStream.Since = since
		peprStream.Timestamps = timestamps

		// Start the stream
		if err := peprStream.Start(context.TODO()); err != nil {
			return fmt.Errorf("Error streaming Pepr logs")
		}
		return nil
	},
}

func init() {
	Cmd.AddCommand(peprCmd)

	peprCmd.Flags().BoolVarP(&follow, "follow", "f", false, lang.CmdPeprMonitorFollowFlag)
	peprCmd.Flags().BoolVarP(&timestamps, "timestamps", "t", false, lang.CmdPeprMonitorTimestampFlag)
	peprCmd.Flags().DurationVar(&since, "since", since, lang.CmdPeprMonitorSinceFlag)
	peprCmd.Flags().BoolVar(&json, "json", false, lang.CmdPeprMonitorJSONFlag)
}
