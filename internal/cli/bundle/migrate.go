// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

const migrationPrompt = `Read and follow the repository's Legacy-to-Next migration skill at
.agents/skills/migrate-legacy-bundle-to-next/SKILL.md. If that file is unavailable,
read the skill at:
https://raw.githubusercontent.com/defenseunicorns/uds-cli/main/.agents/skills/migrate-legacy-bundle-to-next/SKILL.md
Read referenced repository documentation from the same upstream repository when
it is not available locally. If required guidance cannot be read, ask me to supply
it before proceeding. Then migrate
uds-bundle.yaml.

Write the proposed Next files in the current working directory. Do not overwrite
existing output files or modify the Legacy files.
For package overrides, inspect package definitions and report every missing or
unverified values mapping. Produce bundle.uds.hcl, the required values files,
defaults.uds.hcl when needed, and a migration report. Report all deployment-time
values and settings as not converted.
Preserve only package-verification policies supplied by the Legacy bundle. Do not
generate commented verification options or infer a policy for packages without one;
report missing policies as blockers for bundle create.
Do not run UDS commands yet. Do not deploy a bundle or perform cluster operations
as part of this migration.`

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
The agent reads the migration skill locally or from its upstream URL; a full
repository checkout is not required. Supply the skill file if network access
is unavailable. Review the generated files and migration-report.md before use.

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
  Migration validation ends at artifact creation; the skill does not deploy
  bundles or perform cluster operations.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(streams.Out(), migrationPrompt)
			return err
		},
	}
}
