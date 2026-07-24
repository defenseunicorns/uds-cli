// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd provides the root command and subcommand registration for the UDS CLI.
package cmd

import (
	"log/slog"
	"os"

	"github.com/defenseunicorns/uds-cli/pkg/cmd/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/tools"
	cmdversion "github.com/defenseunicorns/uds-cli/pkg/cmd/version"
	cmdzarf "github.com/defenseunicorns/uds-cli/pkg/cmd/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
	"github.com/spf13/cobra"
	"github.com/zarf-dev/zarf/src/pkg/ocischeme"
)

// NewRootCommand creates the root command for uds.
func NewRootCommand(streams iostreams.IOStreams) *cobra.Command {
	var logLevel string

	rootCmd := &cobra.Command{
		Use:   "uds",
		Short: "UDS CLI - The entrypoint to the Defense Unicorns ecosystem",
		Long:  `UDS CLI is a command-line tool for managing UDS Bundles and interacting with the Defense Unicorns ecosystem.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			level, err := logger.ParseLevel(logLevel)
			if err != nil {
				return err
			}
			slog.SetDefault(logger.New(os.Stderr, level))
			cmd.SetContext(ocischeme.WithNegotiator(cmd.Context(), ocischeme.New(ocischeme.Options{})))
			return nil
		},
		// TraverseChildren ensures cobra finds the deepest matching subcommand before parsing flags.
		// Without this, flags like "-n" passed to deeply nested commands (e.g., "uds tools zarf tools kubectl -n ...")
		// would be incorrectly parsed by intermediate parent commands.
		TraverseChildren: true,
	}

	rootCmd.SetIn(streams.In())
	rootCmd.SetOut(streams.Out())
	rootCmd.SetErr(streams.ErrOut())

	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().Bool("prompt", false, "enable interactive confirmation prompts")

	rootCmd.AddCommand(cmdversion.NewVersionCommand(streams))
	rootCmd.AddCommand(bundle.NewBundleCommand(streams))
	rootCmd.AddCommand(tools.NewToolsCommand())
	// Hidden root-level zarf command for internal Zarf callbacks.
	// Zarf's ActionsCommandZarfPrefix is set to "zarf" (single word) at build time,
	// so callbacks run as "uds zarf tools kubectl ..." which routes here.
	rootCmd.AddCommand(cmdzarf.NewInternalZarfCommand())

	return rootCmd
}
