// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package tools provides tools and utilities for UDS workflows.
package tools

import (
	cmdzarf "github.com/defenseunicorns/uds-cli/internal/cli/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// NewToolsCommand creates the tools subcommand.
//
// Usage:
//
//	uds tools zarf <zarf-commands>
//	uds tools migrate-bundle skill
func NewToolsCommand(streams iostreams.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "Tools and utilities for UDS workflows",
		Long:  `Provides access to vendored CLI tools and UDS workflow utilities.`,
	}

	cmd.AddCommand(cmdzarf.NewZarfCommand())
	cmd.AddCommand(NewMigrateBundleCommand(streams))

	return cmd
}
