---
name: migrate-legacy-bundle-to-next
description: Convert a Legacy UDS CLI uds-bundle.yaml into reviewed UDS CLI Next HCL files. Use when migrating a bundle authoring workflow; do not use for arbitrary YAML-to-HCL conversion.
---

# Migrate a Legacy bundle to UDS CLI Next

Produce an experimental, reviewable first-pass migration from uds-bundle.yaml.
Preserve the Legacy input. Convert only plainly equivalent behavior and report
uncertainties rather than guessing.

## Maintained documentation

Use these maintained documents as the source of truth for schema and migration
behavior. Read only the sections relevant to the supplied bundle. In the binary's
printed prompt, these links point to documentation included below; no repository
checkout or web access is required. When using the repository skill directly,
read the linked local files. Do not fetch documentation or missing inputs online.

| Need | Maintained document |
| --- | --- |
| Legacy mappings, override approach, and known gaps | [Legacy-to-Next migration](docs/how-to-guides/migrate-legacy-to-next.mdx) |
| Next schema, package sources, dependencies, values, and verification | [Bundle HCL reference](docs/reference/next/bundle-uds-hcl.mdx) |
| Bundle defaults, consumer configuration, and variable precedence | [Configuration HCL reference](docs/reference/next/config-uds-hcl.mdx) |
| Artifact creation and package preparation | [Create a Next bundle](docs/how-to-guides/next/create-a-bundle.mdx) |
| Legacy keyless fields and verification constraints | [Legacy keyless package verification](docs/how-to-guides/verify-keyless-package-signatures.mdx) |
| Package verification versus artifact signing | [Next bundle artifact signing](docs/how-to-guides/next/sign-bundle-artifacts.mdx) |

Documentation examples describe broader manual workflows. They are reference
material, not permission to execute commands. The restrictions in this skill
govern the migration: do not deploy, operate a cluster, publish, or write to a
registry. Do not copy example trust material or verification bypasses.

## Inputs and output safety

Use the current working directory unless the user specifies another location.
Ask for the Legacy bundle path or contents when absent. Check exact output files
for collisions; do not overwrite existing files without explicit approval for a
named continuation. Never merge an unrelated migration or modify Legacy files.

Write bundle.uds.hcl and migration-report.md. Generate values/<package>.yaml only
for safely mapped overrides and defaults.uds.hcl only for safely converted,
portable bundle defaults. Consumer-owned uds-config.yaml settings are a separate
task. Produce files, not just fenced chat output.

For overrides, inspect supplied local package definitions. If required definitions
or other inputs are unavailable, report the gap and request them instead of
guessing. Do not run UDS commands or access registries during initial generation.

## Sensitive values

Treat passwords, tokens, credentials, kubeconfigs, private keys, client-key
material, secret-manager credentials, and credential-bearing connection strings
as sensitive. Honor user and organization classifications. Public keys, package
references, namespaces, signer identities, and public trusted roots are not
sensitive by default.

Never echo sensitive values in chat, reports, or generated comments. Report only
their source locations as needs sensitive-value handling. Ask before copying them
into a specified untracked local file, or offer a redacted migration requiring
manual injection. Do not silently substitute placeholders.

## Conversion guardrails

Use the maintained documentation for exact field names, types, schema, variable
representation, and signing constraints. Use valid HCL/YAML encoding.

- Preserve metadata and valid unique package labels; report invalid or repeated
  labels instead of renaming them.
- Resolve local paths against the Legacy bundle directory and rebase them to the
  output directory. Retain available Zarf package directories or archives. Report
  authoring directories as needs local package preparation and validation; do not
  skip them merely for being unbuilt or guess archive filenames. Report unavailable
  targets as needs local source review. Record local ref values in the report
  because there is no separate Next local-ref field.
- Preserve namespace and optional component selection, removing exact duplicate
  component entries while preserving their order.
- Do not infer dependencies from Legacy list order or imports/exports. Add
  depends_on only for explicitly established dependencies; otherwise report
  needs package-ordering review.
- Migrate only the supplied Legacy publicKey or keylessVerification policy using
  the Legacy and Next references. Preserve literal public-key contents and resolve
  key-file paths against the Legacy bundle directory. Report unavailable keys,
  unsupported fields, and invalid or ambiguous policies.
- If Legacy supplies neither policy, omit signature_verification and report
  needs package-verification policy as a create-time blocker. Do not generate
  empty blocks, commented alternatives, placeholder trust, or verify = false.
- Convert overrides only with inspected, unambiguous Zarf chart mappings between
  package values sourcePath and Helm targetPath. Preserve component/chart
  attribution and effective YAML types, including static null. Convert simple
  scalar defaults only when their mapping and Next representation are established.
  Do not invent mappings or combine conflicting charts.
- Report missing mappings, uninspectable valuesFiles, chart-specific namespaces,
  interpolation, merges, collection/file variables, configurable nulls, ambiguous
  coercion, and direct Zarf-variable behavior for review rather than guessing.
- Report unsupported fields and deployment-time settings, including build, extra
  metadata, package description/timeout/flavor, and imports/exports. Do not claim
  equivalent behavior where the documentation does not establish it.

## Migration report and validation

Begin the report with an Experimental migration warning: generated files, sources,
values mappings, variable behavior, and verification settings require review.
A successful artifact build does not prove deployment equivalence.

Account for every Legacy field in a source-attribution table: source location,
generated file or report section, converted/needs review/not converted, and reason.
Keep comments brief and limited to source attribution. Keep the report
self-contained: inline essential explanations and next steps instead of referring
the reader to repository docs, skill files, or web links. Retain supplied input
and generated file paths for attribution.

Review generated HCL against the maintained schema, ensure defaults contain only
valid non-null variables, and list unresolved sources, mappings, trust policies,
sensitive values, and configuration gaps as blockers. In the final response,
list generated paths and blockers without repeating file contents.

Do not run UDS commands yet. If the user later authorizes validation, use
CLI_FEATURES=NextMode=true uds bundle create <output-directory> with exactly one
artifact-signing mode described in the creation/signing documentation. For local
alpha testing, --unsigned leaves the artifact unsigned; it does not disable
package verification. Never infer artifact signing from package verification.

Record validation results and any created artifact path. Stop at artifact
creation. Do not deploy a bundle, operate a cluster, or write to a registry.
