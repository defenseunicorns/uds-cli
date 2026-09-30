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
		Long: `Print a prompt to paste into an AI coding agent for an experimental
Legacy-to-Next bundle migration. Replace uds-bundle.yaml with your
bundle path. The agent writes the proposed files in the current working directory
and checks for existing output files before writing.
The printed prompt includes the canonical skill and its maintained documentation;
no repository checkout, skill file, or web access is required to read them.
Supply local package definitions
when needed to review overrides. Review generated files and migration-report.md.

Example request to your coding agent:
  Replace <path-to-legacy-bundle>/uds-bundle.yaml with your local bundle path.

  Run the command "CLI_FEATURES=NextMode=true uds bundle migrate prompt" and use its
  printed instructions to migrate <path-to-legacy-bundle>/uds-bundle.yaml.

  Write the migrated files and migration report in the current working
  directory. Preserve the Legacy input. Do not run any other UDS commands
  or use a cluster.

Package verification:
  Each package's signature_verification block controls trust in that package.
  The migration preserves Legacy public-key or keyless settings and does not
  generate alternative policies. If Legacy supplies no policy, the block is
  omitted and the report flags a blocker. Next requires a policy for every
  package at create time; resolve missing policies before testing.

Bundle artifact signing:
  Choose exactly one create mode: --signing-key <private-key-or-kms-uri>,
  --keyless (requires an OIDC signing identity), or --unsigned (local alpha).
  --unsigned leaves the bundle unsigned; it does not disable package verification.
  Keep private keys and credentials out of the agent prompt.

Test the generated directory:
  CLI_FEATURES=NextMode=true uds bundle create . --unsigned
  This validates HCL and package policies, retrieves packages, and writes a
  local artifact if successful. It may access registries but does not deploy.
  Reaching package ingestion confirms parsing and create-time validation passed.
  Registry access or local package preparation can still fail afterward.
  Local authoring directories may need to be built into Zarf package archives.
  A successful build does not establish deployment equivalence.
  Migration validation ends at artifact creation; the workflow does not deploy
  bundles or perform cluster operations.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(streams.Out(), migrationPrompt)
			return err
		},
	}
}
