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

	// Bundle-component persistent flags (inherited by all subcommands).
	// Defaults here match ConfigResolver.Defaults() so that flag values
	// can be read directly without needing Changed() checks.
	defaults := NewConfigResolver().Defaults()
	bundleCmd.PersistentFlags().StringP("architecture", "a", defaults.Architecture, "target architecture")
	bundleCmd.PersistentFlags().Bool("plain-http", defaults.PlainHTTP, "use plain HTTP for registry communication")
	bundleCmd.PersistentFlags().Bool("skip-tls-verify", defaults.SkipTLSVerify, "skip TLS certificate verification")
	bundleCmd.PersistentFlags().String("tmp-dir", defaults.TmpDir, "directory for temporary files")
	bundleCmd.PersistentFlags().Int("concurrency", defaults.Concurrency, "degree of parallelism for concurrent operations")
	bundleCmd.PersistentFlags().String("config", "", "path to config.uds.hcl for deploy-time variables and options")
	bundleCmd.PersistentFlags().StringP("output", "o", "text", "output format (text, json, yaml)")

	// Add subcommands
	bundleCmd.AddCommand(NewInspectCommand(streams))
	bundleCmd.AddCommand(NewCreateCommand(streams))
	bundleCmd.AddCommand(NewPushCommand(streams))
	bundleCmd.AddCommand(NewPullCommand(streams))
	bundleCmd.AddCommand(NewDeployCommand(streams))
	bundleCmd.AddCommand(NewRemoveCommand(streams))

	return bundleCmd
}
