// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd provides the root command and subcommand registration for the UDS CLI.
package cmd

import (
	"github.com/defenseunicorns/uds-cli/pkg/cmd/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/tools"
	cmdversion "github.com/defenseunicorns/uds-cli/pkg/cmd/version"
	cmdzarf "github.com/defenseunicorns/uds-cli/pkg/cmd/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// NewRootCommand creates the root command for uds.
func NewRootCommand(streams iostreams.IOStreams) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "uds",
		Short: "UDS CLI - The entrypoint to the Defense Unicorns ecosystem",
		Long:  `UDS CLI is a command-line tool for managing UDS Bundles and interacting with the Defense Unicorns ecosystem.`,
		// TraverseChildren ensures cobra finds the deepest matching subcommand before parsing flags.
		// Without this, flags like "-n" passed to deeply nested commands (e.g., "uds tools zarf tools kubectl -n ...")
		// would be incorrectly parsed by intermediate parent commands.
		TraverseChildren: true,
	}

	rootCmd.AddCommand(cmdversion.NewVersionCommand(streams))
	rootCmd.AddCommand(bundle.NewBundleCommand(streams))
	rootCmd.AddCommand(tools.NewToolsCommand())
	// Hidden root-level zarf command for internal Zarf callbacks.
	// Zarf's ActionsCommandZarfPrefix is set to "zarf" (single word) at build time,
	// so callbacks run as "uds zarf tools kubectl ..." which routes here.
	rootCmd.AddCommand(cmdzarf.NewInternalZarfCommand())

	return rootCmd
}
