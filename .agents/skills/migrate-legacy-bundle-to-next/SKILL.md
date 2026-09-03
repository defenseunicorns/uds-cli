---
name: migrate-legacy-bundle-to-next
description: Convert a Legacy UDS CLI uds-bundle.yaml and optional uds-config.yaml into reviewed UDS CLI Next HCL files. Use when migrating a bundle authoring workflow; do not use for arbitrary YAML-to-HCL conversion.
---

# Migrate a Legacy bundle to UDS CLI Next

Produce a reviewable first-pass migration from `uds-bundle.yaml` and, when supplied,
`uds-config.yaml`. Keep the source files unchanged. The deliverable is not complete
until it calls out every source construct that has no safe Next equivalent.

## Inputs and output

Ask for the legacy bundle and optional config contents or paths. If the bundle uses
`overrides`, also ask for each referenced package's `zarf.yaml` when it is available;
the package mappings determine whether a generated values file is usable.

Return all of the following:

1. `bundle.uds.hcl`, with `uds { bundle_api_version = "uds.dev/v1alpha1" }`, metadata,
   package blocks, and package verification posture.
2. A package-level `values/<package>.yaml` for each safely transcribed legacy override,
   plus the corresponding `values_files` entry. Preserve YAML value types and render
   legacy override variables as `{{ .vars.<package>.<variable> }}`.
3. `config.uds.hcl` for deploy-time variables and options. Generate
   `defaults.uds.hcl` only for values the user identifies as portable build-time
   defaults; it may contain only `variables`, never `options`.
4. A migration report listing converted fields, manual work, unsupported features,
   and the exact Next commands to use.

Use fenced blocks titled with their filenames. Do not claim that generated files were
validated or that an override will work unless the required Zarf mapping was supplied
and checked.

## Safe mappings

Apply these mappings when the source has the required values:

| Legacy | Next |
| --- | --- |
| `metadata.name`, `description`, `version` | `metadata` block fields |
| package `name` | `package "<name>"` label |
| `repository` plus `ref` | `source = "oci://<repository>:<ref>"` |
| local package `path` | `source = "<path>"`; retain `ref` only in the report because it has no separate Next field |
| `namespace` | package `namespace` |
| `optionalComponents` | `optional_components` |
| `publicKey` | `signature_verification { public_key = file("...") }` when the value is a path; preserve literal key content as an HCL string only when it is clearly intended as content |
| `keylessVerification` | `signature_verification { keyless { ... } }`, changing camelCase keys to the documented snake_case keys |
| static override `values` | nested YAML at each override `path` |
| override `variables` | nested YAML at each `path` using `{{ .vars.<package>.<normalized_name> }}`; put defaults/config values under that package in HCL |
| `options.architecture`, `log_level`, `tmp_dir` | same-name fields in `config.uds.hcl` `options` |
| `options.oci_concurrency` | `options.concurrency` |
| legacy `insecure` | manual decision between `plain_http` and `skip_tls_verify`; do not choose automatically |

Normalize legacy override variable names to lowercase snake case (for example,
`REPLICA_COUNT` becomes `replica_count`) consistently in values files and HCL.
Use package-scoped variables for values-file templates. Only top-level scalar values
are passed through to Zarf package-variable substitutions.

Every Next package must declare one verification posture. Preserve a legacy public-key
or keyless configuration. If the legacy package has neither, do not silently disable
verification: emit an unresolved `signature_verification` TODO and list it as a
blocking manual decision. Offer `verify = false` only as an explicitly labelled,
security-reducing local-alpha option.

## Override review

Legacy overrides target a component and chart; Next values files are package-level.
For each override, retain the legacy component/chart location in the migration report.
Check that the target package's `zarf.yaml` maps every generated values-file path to
the intended chart value. If `zarf.yaml` is absent, a mapping is missing, two charts
write conflicting paths, a legacy `valuesFiles` path cannot be inspected, or an
override sets a chart-specific namespace, generate the proposed file only when its
content is unambiguous and mark it **needs Zarf mapping review**. Never say it is
equivalent until that review passes.

## Always report these gaps

Flag rather than drop any occurrence of:

- bundle `kind` and `build` metadata;
- metadata `architecture` (move it to `config.uds.hcl`), `uncompressed`, URL,
  authors, documentation, source, vendor, or aggregate checksum;
- package `description`, `timeout`, `flavor`, `imports`, and `exports`;
- legacy `valuesFiles` that cannot be folded into a package values file and verified
  against Zarf mappings;
- `shared` configuration, `uds_cache`, `retries`, `UDS_<NAME>` environment variables,
  and `--set` workflows;
- legacy command/flag behavior without a Next equivalent, including `uds logs`,
  `uds list`, deploy `--resume`, `--retries`, `--force-conflicts`, and inspect
  `--sbom`, `--list-images`, and `--list-variables`.

`imports` and `exports` have no direct Next equivalent. Explain the affected values
must instead be supplied through `config.uds.hcl` or values files; do not translate
them to `depends_on` unless the user independently establishes an ordering dependency.

## Commands and final review

Use these command changes in the report:

| Legacy | Next |
| --- | --- |
| `uds create` | `CLI_FEATURES=NextMode=true uds bundle create` |
| `uds deploy` | `CLI_FEATURES=NextMode=true uds bundle deploy` |
| `uds dev deploy` | `CLI_FEATURES=NextMode=true uds bundle dev deploy` |
| `uds inspect` | `CLI_FEATURES=NextMode=true uds bundle inspect` |
| `uds publish` / `uds pull` / `uds remove` | `uds bundle push` / `uds bundle pull` / `uds bundle remove`, each with `CLI_FEATURES=NextMode=true` |
| `uds zarf` | `CLI_FEATURES=NextMode=true uds tools zarf` (except vendored tools remain `uds zarf tools <tool>`) |

Recommend a non-production `CLI_FEATURES=NextMode=true uds bundle dev deploy` before
creating and signing the artifact. Next artifacts use `.tar.zst`; source definitions
and artifacts are not backward compatible. Point the user to
`docs/how-to-guides/migrate-legacy-to-next.mdx` in this repository (or the published
Migration guide) for the maintained human walkthrough.
