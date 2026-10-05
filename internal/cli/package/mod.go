// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packagecli

import (
	"os"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
)

type commonOptions struct {
	PlainHTTP     bool
	SkipTLSVerify bool
	TmpDir        string
	CachePath     string
	Concurrency   int
	LogLevelName  string
}

// NewModCommand creates the package modification parent command.
func NewModCommand(streams iostreams.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mod",
		Short: "Modify Zarf packages",
	}

	cmd.PersistentFlags().Bool("plain-http", false, "allow plain HTTP when a registry does not support HTTPS")
	cmd.PersistentFlags().Bool("skip-tls-verify", false, "skip TLS certificate verification")
	cmd.PersistentFlags().String("tmp-dir", os.TempDir(), "directory for temporary files")
	cmd.PersistentFlags().String("cache", "", "directory to cache images and Git repositories")
	cmd.PersistentFlags().Int("concurrency", zoci.DefaultConcurrency, "degree of parallelism for concurrent operations")

	cmd.AddCommand(NewDisassembleCommand(streams))
	cmd.AddCommand(NewReassembleCommand(streams))
	return cmd
}

func completeCommonOptions(cmd *cobra.Command, opts *commonOptions) error {
	var err error
	if opts.PlainHTTP, err = cmd.Flags().GetBool("plain-http"); err != nil {
		return err
	}
	if opts.SkipTLSVerify, err = cmd.Flags().GetBool("skip-tls-verify"); err != nil {
		return err
	}
	if opts.TmpDir, err = cmd.Flags().GetString("tmp-dir"); err != nil {
		return err
	}
	if opts.CachePath, err = cmd.Flags().GetString("cache"); err != nil {
		return err
	}
	if opts.Concurrency, err = cmd.Flags().GetInt("concurrency"); err != nil {
		return err
	}
	opts.LogLevelName = "info"
	if cmd.Flags().Lookup("log-level") != nil {
		opts.LogLevelName, err = cmd.Flags().GetString("log-level")
	}
	return err
}
