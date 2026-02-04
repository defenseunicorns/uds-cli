// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"github.com/defenseunicorns/uds-cli/pkg/cmd/bundle"
	cmdversion "github.com/defenseunicorns/uds-cli/pkg/cmd/version"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// NewRootCommand creates the root command for uds-cli-next.
func NewRootCommand(streams iostreams.IOStreams) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "uds-cli-next",
		Short: "UDS CLI Next - A tool for managing UDS Bundles",
		Long:  `UDS CLI Next is a command-line tool for creating, pushing, pulling, deploying, and removing UDS Bundles.`,
	}

	rootCmd.AddCommand(cmdversion.NewVersionCommand(streams))
	rootCmd.AddCommand(bundle.NewBundleCommand(streams))

	return rootCmd
}
