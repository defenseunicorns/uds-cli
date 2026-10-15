// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package packagecli provides CLI commands for managing Zarf packages.
package packagecli

import (
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// NewPackageCommand creates the package parent command.
func NewPackageCommand(streams iostreams.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "package",
		Short: "Manage Zarf packages",
	}
	cmd.AddCommand(NewModCommand(streams))
	return cmd
}
