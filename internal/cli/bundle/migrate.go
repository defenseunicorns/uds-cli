// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

const migrationPrompt = `Migrate uds-bundle.yaml from Legacy UDS CLI to a reviewable first-pass
UDS CLI Next bundle using the instructions below. These instructions are
self-contained; do not fetch a skill, repository, or documentation from the web.
If required package definitions or other inputs are unavailable locally, report
the gap and ask me to supply them rather than guessing.

Scope and output:
- Preserve the Legacy input. Write proposed Next files in the current working
  directory unless I specify another location. Check exact output files for
  collisions before writing; do not overwrite existing output without approval.
- Produce bundle.uds.hcl and migration-report.md. Generate values/<package>.yaml
  only for safely mapped overrides, and defaults.uds.hcl only for safely converted
  portable bundle defaults. Do not copy consumer-owned uds-config.yaml settings.
- Begin the report with an Experimental migration warning: generated files,
  sources, values mappings, variable behavior, and verification settings require
  review. A successful build does not prove deployment equivalence.
- Never echo sensitive values in chat, reports, or comments. Report their source
  locations as needs sensitive-value handling and ask before copying them into
  a specified untracked local file. Public keys and signer identities are not
  sensitive by default. Do not silently substitute placeholders for secrets.

Next bundle structure:
uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

Use a metadata block with the Legacy name, description, and version as strings.
For each package, use package "<legacy-name>" { ... }. Labels must be unique,
cannot contain / or backslash, and cannot be either "." or "..". Report invalid or repeated
labels instead of renaming them. Use valid HCL/YAML encoding, including escaping
literal template delimiters and backslashes when needed.

Package mappings:
- repository plus ref becomes source = "oci://<repository>:<ref>".
- Resolve local paths from the Legacy bundle's directory, then express the same
  target relative to the generated bundle. Retain available local Zarf package
  directories or .tar.zst archives. Report authoring directories as needs local
  package preparation and validation; do not omit them merely for being unbuilt.
  Report unavailable local targets as needs local source review. A local ref has
  no separate Next field; retain it in the report.
- namespace becomes namespace. optionalComponents becomes optional_components,
  removing exact duplicates while preserving order.
- Add depends_on = [package.<id>] only for an explicitly established dependency.
  Legacy list order is not a dependency graph. Otherwise report ordering review.

Package verification:
- Migrate only the supplied Legacy publicKey or keylessVerification policy into
  signature_verification. Do not add commented alternatives or infer trust.
- publicKey becomes public_key. Preserve literal PEM content as an HCL string or
  heredoc. For a key-file path, resolve the original file and use file("<path>")
  with the path relative to the generated bundle. Report unavailable key files.
- keylessVerification becomes a nested keyless block. Map these field names:
  certificateIdentity -> certificate_identity
  certificateIdentityRegexp -> certificate_identity_regexp
  certificateOIDCIssuer -> certificate_oidc_issuer
  certificateOIDCIssuerRegexp -> certificate_oidc_issuer_regexp
  trustedRoot -> trusted_root
  insecureIgnoreTlog -> insecure_ignore_tlog
  useSignedTimestamps -> use_signed_timestamps
  Preserve supplied values and report unsupported or ambiguous fields. Keyless
  needs exactly one identity constraint and one issuer constraint. Enabled
  verification needs exactly one public_key or keyless policy.
- If Legacy supplies neither policy, omit signature_verification and report
  needs package-verification policy as a create-time blocker. Do not add an empty
  block, placeholder trust material, or verify = false. Missing trust material
  does not authorize a verification bypass.

Helm overrides and defaults:
- Legacy overrides address a component/chart; Next values files are package-level.
  Inspect supplied local zarf.yaml definitions. Every converted value must have
  an unambiguous chart values mapping from sourcePath (package values YAML) to
  targetPath (the intended Helm value). Preserve component/chart attribution.
- With a verified mapping, write static values at the mapped YAML paths and add
  values_files = ["values/<package>.yaml"] to the package. Preserve YAML types,
  including static null. Do not invent mappings or combine conflicting charts.
- For a mapped simple scalar override default, normalize the variable name to
  lowercase snake_case. Use a package-scoped Go template in the values file,
  such as {{ .vars.app.replica_count }}, and a matching defaults.uds.hcl entry:
  variables = { app = { replica_count = 2 } }
  Use template index access for package names that are not valid dot identifiers.
  defaults.uds.hcl may contain only variables, never options or null variables.
- Report missing mappings, uninspectable valuesFiles, chart-specific namespaces,
  interpolation, merges, collection/file variables, configurable nulls, ambiguous
  coercion, and direct Zarf-variable behavior as review items; do not guess.
  Only top-level scalar variables pass through to Zarf variable substitutions.

Report and validation:
- Account for every Legacy field in a source-attribution table: source location,
  generated file or report section, converted/needs review/not converted, reason.
  Report unsupported fields, including kind/build, extra metadata, package
  description/timeout/flavor, imports/exports, and deployment-time settings.
  Imports/exports have no direct equivalent; do not turn them into dependencies.
- Review generated HCL against the structure above and list all unresolved
  sources, mappings, trust policies, sensitive values, and configuration gaps.
- Do not run UDS commands yet. If I later authorize validation, use
  CLI_FEATURES=NextMode=true uds bundle create . with exactly one artifact-signing
  mode: --signing-key <private-key-or-kms-uri>, --keyless, or --unsigned for a
  local alpha test. Use the specified output directory instead of . if changed.
  Artifact signing is independent of package verification; --unsigned leaves the
  finished artifact unsigned and does not disable package verification.
- Record validation results and any created artifact path. Stop at artifact
  creation. Do not deploy a bundle, operate a cluster, or write to a registry.`

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
The printed prompt includes the migration instructions; no repository checkout,
skill file, or web access is required to read them. Supply local package definitions
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
			_, err := fmt.Fprintln(streams.Out(), migrationPrompt)
			return err
		},
	}
}
