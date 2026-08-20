// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package tools provides the tools subcommand that groups vendored CLI tools.
package tools

import (
	cmdzarf "github.com/defenseunicorns/uds-cli/internal/cli/zarf"
	"github.com/spf13/cobra"
)

// NewToolsCommand creates the tools subcommand that groups vendored CLI tools.
//
// Usage:
//
//	uds tools zarf <zarf-commands>
func NewToolsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "Collection of vendored CLI tools",
		Long:  `Provides access to vendored CLI tools such as Zarf.`,
	}

	cmd.AddCommand(cmdzarf.NewZarfCommand())

	return cmd
}
