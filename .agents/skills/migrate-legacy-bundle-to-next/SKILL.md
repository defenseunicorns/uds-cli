---
name: migrate-legacy-bundle-to-next
description: Convert a Legacy UDS CLI uds-bundle.yaml into reviewed UDS CLI Next HCL files. Use when migrating a bundle authoring workflow; do not use for arbitrary YAML-to-HCL conversion.
---

# Migrate a Legacy bundle to UDS CLI Next

Produce an experimental, reviewable first-pass migration from `uds-bundle.yaml`.
This is not a compatibility guarantee or a compiler: convert only what is plainly
equivalent and report every uncertainty for human review. Do not modify Legacy input.

Begin the migration report with an **Experimental migration warning**: generated
files, package sources, values mappings, variable behavior, and verification settings
all require review before use in any environment. A successful artifact build does
not prove deployment equivalence.

## Read maintained documentation as needed

Repository documentation is the source of truth for Next behavior. Read only the
documents relevant to the migration at hand:

| Need | Read |
| --- | --- |
| Legacy-to-Next mappings and override approach | `docs/how-to-guides/migrate-legacy-to-next.mdx` |
| Next bundle schema, package labels, sources, values files, and package verification | `docs/reference/next/bundle-uds-hcl.mdx` |
| Bundle defaults versus consumer deploy-time configuration | `docs/reference/next/config-uds-hcl.mdx` |
| Create workflow and dependency behavior | `docs/how-to-guides/next/create-a-bundle.mdx` |
| Source development deployment versus artifact deployment | `docs/how-to-guides/next/deploy-a-bundle.mdx` |
| Package keyless verification and bundle artifact signing | `docs/how-to-guides/verify-keyless-package-signatures.mdx` and `docs/how-to-guides/next/sign-bundle-artifacts.mdx` |

If the documentation does not establish an exact, safe conversion, do not infer one:
leave that item unconverted and report it.

## Inputs, discovery, and output safety

Ask for the Legacy bundle path or contents and the output directory. For a new
migration, stop if that output directory already exists; ask the user to move or
remove it. Only edit an existing output when the user explicitly authorizes a named
continuation. Never merge an unrelated prior migration or modify Legacy files.

For a package with overrides, inspect its available local definition. For an OCI
package, inspect its definition when commands and network access are permitted:

```sh
CLI_FEATURES=NextMode=true uds tools zarf package inspect definition 'oci://<repository>:<ref>'
```

This read-only inspection may use existing registry credentials but must not write to
a registry or operate a cluster. Respect an explicit instruction not to run commands
or access the network. If inspection is unavailable, report the values mapping as
unverified; ask for the relevant `zarf.yaml` only when needed to resolve an override.

Write these files directly under the requested output directory:

1. `bundle.uds.hcl`
2. `values/<package>.yaml` only for safely converted overrides
3. `defaults.uds.hcl` only for safely converted bundle defaults
4. `migration-report.md`

Do not substitute fenced chat output for files. In the final response, list generated
paths and unresolved blockers without repeating file contents.

## Sensitive values

Treat passwords, tokens, credentials, kubeconfigs, private keys, client-key material,
secret-manager credentials, and credential-bearing connection strings as sensitive.
Honor user or organization classifications. Public package references, namespaces,
public keys, keyless certificate identities, and public trusted roots are not
sensitive by default.

Never echo a sensitive value in chat, report prose, or generated-file comments. Report
only its source location and **needs sensitive-value handling**. Before copying it,
ask the user to authorize a specified untracked local file or choose a redacted
migration requiring manual injection. Do not silently replace it with a placeholder.

## Conversion scope

Use valid HCL and YAML encoding. Preserve effective value types only when their
representation is unambiguous; otherwise report the item instead of guessing.

| Legacy construct | First-pass handling |
| --- | --- |
| `metadata.name`, `description`, `version` | Convert to the `metadata` block. |
| `repository` and `ref` | Convert to `source = "oci://<repository>:<ref>"`. |
| local `.tar.zst` path | Use the equivalent path relative to the generated bundle. |
| local directory path | Use only its resolved Legacy archive path. If the archive or target architecture is unavailable, report **needs local package preparation**; do not use a bare authoring directory. |
| unique valid package name | Use as the package label. |
| invalid or repeated package name | Report **needs package-label review**; do not silently rename it. |
| `namespace` | Convert to package `namespace`. |
| `optionalComponents` | Convert to `optional_components`, removing exact duplicates while preserving order. |
| `publicKey` or `keylessVerification` | Preserve the package verification posture using the Next reference. Never silently add `verify = false`. |
| no Legacy package verification posture | Add an all-commented, user-choice `signature_verification` block with key-based, keyless, and explicitly local-alpha `verify = false` options. Do not uncomment a choice. |
| static override value with a verified, unambiguous Zarf mapping | Write the mapped YAML value and add `values_files`. Static YAML `null` remains YAML `null`. |
| simple scalar override default with a verified mapping and valid Next representation | Write a values-file template and the bundle default in `defaults.uds.hcl`. Do not write null configuration variables. |
| anything else | Do not convert it; report why and what information or decision is required. |

The following are review-required by default: `${NAME}` interpolation; values-file
merges; list, object, or file variables; configurable nulls; template delimiters;
ambiguous scalar coercion; conflicting or shared chart mappings; direct Zarf-variable
use; package ordering assumptions; deploy-time settings; and any source, label, or
trust decision whose exact Next equivalent is not established by the documentation and
available package definition.

Do not copy Legacy `uds-config.yaml` values or options into generated bundle files.
`defaults.uds.hcl` contains only portable bundle defaults; consumer-owned deployment
configuration remains outside this first-pass migration.

## Trust, sources, and dependencies

Keep package verification and bundle-artifact signing as separate user decisions.
When verification material or a signing identity is unavailable, preserve the
verification requirement and report validation as blocked. A local-alpha
`verify = false` change is allowed only after explicit user authorization, only in a
separate validation copy, and must be labelled security-reducing and non-equivalent.

Do not treat Legacy package list order as a Next dependency graph. Add `depends_on`
only when the user or source clearly establishes the dependency and the resulting
package references are valid. Otherwise report **needs package-ordering review**.

For verification-sensitive validation, prefer deploying the exact artifact created
from the migrated directory. Record the resulting local artifact path, or an immutable
published OCI reference, in the report and use `uds bundle deploy` with the appropriate
artifact verification inputs. Source `uds bundle dev deploy` is a separate,
non-production authoring workflow: it can reload mutable package sources and does not
validate the created artifact.

## Migration report and validation

Include a concise source-attribution table with the Legacy location, generated file
or report section, disposition (`converted`, `needs review`, or `not converted`), and
reason for every converted block and blocker. Keep generated comments brief and use
them only to identify source ranges; do not make an unverified mapping look approved.

Before reporting the migration as ready for review:

- check each generated HCL file against the Next schema;
- ensure `defaults.uds.hcl` contains only valid non-null variables;
- ensure each generated values path has an inspected or supplied Zarf mapping;
- ensure every package has an explicit, user-reviewed verification posture; and
- list unavailable archives, missing mappings, sensitive values, trust choices, and
  deployment-time settings as blockers rather than claiming equivalence.

When commands are authorized, follow the linked create and signing guides to validate
the generated directory. Do not run a cluster operation without explicit user
approval. Never recommend artifact deployment with signature verification bypassed
unless the user explicitly selects the documented local-alpha exception.
