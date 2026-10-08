// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package tools

import (
	_ "embed"
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// migrationSkill is generated from the canonical migration skill.
//
//go:embed assets/migration-skill.md
var migrationSkill string

// NewMigrateBundleCommand creates commands that assist with Legacy bundle migration.
func NewMigrateBundleCommand(streams iostreams.IOStreams) *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "migrate-bundle",
		Short: "Get help migrating Legacy bundles to Next",
	}

	migrateCmd.AddCommand(NewMigrationSkillCommand(streams))
	return migrateCmd
}

// NewMigrationSkillCommand creates the migration skill command.
func NewMigrationSkillCommand(streams iostreams.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the Legacy-to-Next migration skill",
		Long: `Print the canonical Legacy-to-Next migration skill and its included
documentation for an AI coding agent. This command only prints text;
it does not migrate files or run bundle operations. No repository checkout or
web access is required to obtain the instructions.

Paste the output into your agent and provide the path to your Legacy bundle,
or ask the agent to run this command as shown below.

Experimental: the agent produces a first-pass proposal, not a compatibility
guarantee. Review the generated files and migration report before use.

Example request to your coding agent:
  Replace <path-to-legacy-bundle>/uds-bundle.yaml with your local bundle path.

  Run the command "CLI_FEATURES=NextMode=true uds tools migrate-bundle skill" and use its
  printed instructions to migrate <path-to-legacy-bundle>/uds-bundle.yaml.

  Write the migrated files and migration report in the current working
  directory. Preserve the Legacy input. Do not run any other UDS commands
  or use a cluster.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(streams.Out(), migrationSkill)
			return err
		},
	}
}
