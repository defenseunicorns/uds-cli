// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle provides CLI commands for managing UDS bundles.
package bundle

import (
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// NewBundleCommand creates the bundle parent command.
func NewBundleCommand(streams iostreams.IOStreams) *cobra.Command {
	bundleCmd := &cobra.Command{
		Use:   "bundle",
		Short: "Manage UDS bundles",
		Long:  "Manage UDS bundles, which are collections of resources that can be created, deployed, and removed together",
	}

	// Add subcommands
	bundleCmd.AddCommand(NewInspectCommand(streams))
	bundleCmd.AddCommand(NewCreateCommand(streams))
	bundleCmd.AddCommand(NewPushCommand(streams))
	bundleCmd.AddCommand(NewPullCommand(streams))
	bundleCmd.AddCommand(NewDeployCommand(streams))
	bundleCmd.AddCommand(NewRemoveCommand(streams))

	return bundleCmd
}
