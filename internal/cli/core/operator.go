// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package core

import (
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// NewOperatorCommand creates the operator resource command.
func NewOperatorCommand(streams iostreams.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Manage and observe the UDS Core operator",
		Long:  "Manage and observe UDS Core operator activity",
	}

	cmd.AddCommand(NewOperatorMonitorCommand(streams))

	return cmd
}
