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
		Long: `Print a prompt to paste into an AI coding agent for an experimental
Legacy-to-Next bundle migration. Replace ./legacy/uds-bundle.yaml with your
bundle path and use a new output directory (the prompt defaults to ./.next).
The agent reads the migration skill locally or from its upstream URL; a full
repository checkout is not required. Supply the skill file if network access
is unavailable. Review the generated files and migration-report.md before use.

Package verification:
  Each package's signature_verification block controls trust in that package.
  Preserve migrated public_key or keyless settings. For an unresolved choice,
  select the package signer's public key or keyless identity and OIDC issuer.
  For local alpha testing only, you may explicitly select verify = false;
  this includes an unverified package. Do not combine it with a trust method.

Bundle artifact signing:
  Choose exactly one create mode: --signing-key <private-key-or-kms-uri>,
  --keyless (requires an OIDC signing identity), or --unsigned (local alpha).
  --unsigned leaves the bundle unsigned; it does not disable package verification.
  Keep private keys and credentials out of the agent prompt.

Test the generated directory:
  CLI_FEATURES=NextMode=true uds bundle create ./.next --unsigned
  This validates HCL and package policies, retrieves packages, and writes a
  local artifact if successful. It may access registries but does not deploy.
  Reaching package ingestion confirms parsing and create-time validation passed.
  Registry access or local package preparation can still fail afterward.
  Local authoring directories may need to be built into Zarf package archives.
  A successful build does not establish deployment equivalence.

Deployment:
  Use a non-production cluster that meets the packages' requirements, including
  Zarf initialization for standard packages. Package list order does not set
  dependencies. Signed bundle artifacts need matching verification inputs.
  Deploying an unsigned local alpha artifact requires the explicit
  --skip-signature-verification bypass for bundle integrity verification.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(streams.Out(), migrationPrompt)
			return err
		},
	}
}
