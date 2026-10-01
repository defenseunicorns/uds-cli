// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	_ "embed"
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// migrationPrompt is generated from the canonical migration skill.
//
//go:embed assets/migration-prompt.md
var migrationPrompt string

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
		Long: `Print the canonical Legacy-to-Next migration skill and its included
documentation as a prompt for an AI coding agent. This command only prints text;
it does not migrate files or run bundle operations. No repository checkout or
web access is required to obtain the instructions.

Paste the output into your agent and provide the path to your Legacy bundle,
or ask the agent to run this command as shown below.

Experimental: the agent produces a first-pass proposal, not a compatibility
guarantee. Review the generated files and migration report before use.

Example request to your coding agent:
  Replace <path-to-legacy-bundle>/uds-bundle.yaml with your local bundle path.

  Run the command "CLI_FEATURES=NextMode=true uds bundle migrate prompt" and use its
  printed instructions to migrate <path-to-legacy-bundle>/uds-bundle.yaml.

  Write the migrated files and migration report in the current working
  directory. Preserve the Legacy input. Do not run any other UDS commands
  or use a cluster.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(streams.Out(), migrationPrompt)
			return err
		},
	}
}
