// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

const migrationPrompt = `Read and follow the repository's Legacy-to-Next migration skill at
.agents/skills/migrate-legacy-bundle-to-next/SKILL.md. Then migrate
./legacy/uds-bundle.yaml.

Write only the proposed Next files under ./.next. Do not modify the Legacy files.
For package overrides, inspect package definitions and report every missing or
unverified values mapping. Produce bundle.uds.hcl, the required values files,
defaults.uds.hcl when needed, and a migration report. Report all deployment-time
values and settings as not converted.
Do not run UDS commands or perform a cluster operation yet.`

// NewMigrateCommand creates commands that assist with Legacy bundle migration.
func NewMigrateCommand(streams iostreams.IOStreams) *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Get help migrating Legacy bundles to Next",
	}

	migrateCmd.AddCommand(NewMigrationPromptCommand(streams))
	return migrateCmd
}

// NewMigrationPromptCommand creates the migration prompt command.
func NewMigrationPromptCommand(streams iostreams.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:   "prompt",
		Short: "Print the Legacy-to-Next migration prompt",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(streams.Out(), migrationPrompt)
			return err
		},
	}
}
