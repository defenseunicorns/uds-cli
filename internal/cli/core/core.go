// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package core provides CLI commands for UDS Core operations.
package core

import (
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// NewCoreCommand creates the core parent command.
func NewCoreCommand(streams iostreams.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "core",
		Short: "Manage and observe UDS Core",
		Long:  "Manage and observe UDS Core resources and operations",
	}

	cmd.AddCommand(NewOperatorCommand(streams))

	return cmd
}
