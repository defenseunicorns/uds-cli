// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package tools provides UDS utilities and vendored CLI tools.
package tools

import (
	cmdzarf "github.com/defenseunicorns/uds-cli/internal/cli/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// NewToolsCommand creates the tools subcommand for UDS utilities and vendored CLI tools.
//
// Usage:
//
//	uds tools zarf <zarf-commands>
func NewToolsCommand(streams iostreams.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "UDS utilities and vendored CLI tools",
		Long:  `Provides UDS utilities and access to vendored CLI tools such as Zarf.`,
	}

	cmd.AddCommand(cmdzarf.NewZarfCommand())
	cmd.AddCommand(newClearCacheCommand(streams))

	return cmd
}
