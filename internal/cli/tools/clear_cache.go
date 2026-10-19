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
Use --uds-cache to select the cache directory, or --config to read options.uds_cache.
Use --prompt to request confirmation before removing cache content.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmdbundle.SnapshotFlags(cmd)
			// Cleanup has no bundle source or adjacent defaults.
			config, _, err := resolver.Resolve(cmd.Context(), streams, flags, "")
			if err != nil {
				return err
			}
			s := logger.Bind(streams, config.Options.LogLevel)
			if flags.Prompt && config.Options.CacheDir != "" {
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
	// Keep flags local so Zarf passthrough receives its own flags unchanged.
	cmd.Flags().String("uds-cache", resolver.Defaults().CacheDir, "cache directory (empty disables caching)")
	cmd.Flags().String("config", "", "path to config.uds.hcl for cache directory and logging options")
	return cmd
}
