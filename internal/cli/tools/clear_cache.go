// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package tools

import (
	"fmt"

	cmdbundle "github.com/defenseunicorns/uds-cli/internal/cli/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

func newClearCacheCommand(streams iostreams.IOStreams) *cobra.Command {
	resolver := cmdbundle.NewConfigResolver()
	cmd := &cobra.Command{
		Use:   "clear-cache",
		Short: "Clear the Next bundle OCI cache",
		Long: `Remove content beneath <uds-cache>/bundle-oci, preserving the cache
directory, Legacy's layers cache, and Zarf's default images cache.
Do not run this command while other operations are using the Next cache.
Use --prompt to request confirmation before removing cache content.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmdbundle.SnapshotFlags(cmd)
			// Cleanup has no bundle source or adjacent defaults. Resolve exactly
			// the same default, explicit config, and CLI cache override as bundle reads.
			config, _, err := resolver.Resolve(cmd.Context(), streams, flags, "")
			if err != nil {
				return err
			}
			s := logger.Bind(streams, config.Options.LogLevel)
			if flags.Prompt {
				confirmed, err := cmdbundle.PromptConfirmation(s, fmt.Sprintf("Clear Next cache content under %q?", oci.BundleCachePath(config.Options.CacheDir)))
				if err != nil {
					return err
				}
				if !confirmed {
					s.Debug("cache cleanup cancelled")
					return nil
				}
			}
			return oci.ClearBundleCache(cmd.Context(), config.Options.CacheDir, s)
		},
	}
	// Keep UDS options on this command rather than the tools parent: Zarf
	// passthrough must continue receiving its own flags unchanged.
	cmd.Flags().String("uds-cache", resolver.Defaults().CacheDir, "Specify the location of the UDS cache directory")
	cmd.Flags().String("config", "", "path to config.uds.hcl for cache directory and logging options")
	return cmd
}
