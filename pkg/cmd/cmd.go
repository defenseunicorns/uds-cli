// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"github.com/defenseunicorns/uds-cli/pkg/cmd/bundle"
	cmdversion "github.com/defenseunicorns/uds-cli/pkg/cmd/version"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// NewRootCommand creates the root command for uds.
func NewRootCommand(streams iostreams.IOStreams) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "uds",
		Short: "UDS CLI - The entrypoint to the Defense Unicorns ecosystem",
		Long:  `UDS CLI is a command-line tool for managing UDS Bundles and interacting with the Defense Unicorns ecosystem.`,
	}

	rootCmd.AddCommand(cmdversion.NewVersionCommand(streams))
	rootCmd.AddCommand(bundle.NewBundleCommand(streams))

	return rootCmd
}
