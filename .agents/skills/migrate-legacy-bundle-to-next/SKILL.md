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

Before writing any output, check whether the requested output directory already
exists. If it does, stop without reading, merging, changing, or deleting anything in
that directory. Ask the user to remove or move the existing directory, then wait for
confirmation that the requested output directory is absent. Do not select a different
output directory or overwrite a prior migration automatically.

For every local package `path`, inspect the target when it is available. Determine
whether it is a canonical Zarf package source: either a `.tar.zst` archive or a
package directory that includes the generated `checksums.txt`. A directory containing
only authoring inputs such as `zarf.yaml` needs package preparation before Next can
create a bundle from it.

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

## Source attribution

Make every migration reviewable against its Legacy input. Count source lines from the
input file exactly as provided and use one-based `path:line` or `path:start-end`
references. Do not guess a location when an input is incomplete or generated; say so
in the migration report instead.

Add concise comments immediately before generated semantic blocks, not every
attribute. Attribute each `metadata`, `package`, `signature_verification`, `options`,
and package values section that came from a distinct Legacy source range. For example:

```hcl
# Migrated from uds-bundle.yaml:12-18
package "podinfo" {
  source = "oci://registry.example.com/acme/podinfo:1.2.3"
}
```

Use `# Migrated from ...` comments in HCL and YAML. Keep the original
component/chart location in the comment or nearby migration report entry for a
generated values section. Comments must not conceal manual decisions or make an
unverified Zarf mapping appear validated.

Include a source-attribution table in the migration report with a row for every
converted block and every warning or unsupported field. Each row must contain the
Legacy location, generated file and section (when any), disposition (`converted`,
`needs review`, or `not converted`), and a concise reason. This table is the complete
trace; generated-file comments are navigational aids.

## Safe mappings

Apply these mappings when the source has the required values:

| Legacy | Next |
| --- | --- |
| `metadata.name`, `description`, `version` | `metadata` block fields |
| unique package `name` | `package "<name>"` label |
| repeated package `name` | unique collision-free instance labels, allocated in Legacy source order; see **Repeated package names** |
| `repository` plus `ref` | `source = "oci://<repository>:<ref>"` |
| local package `path` ending in `.tar.zst` | `source = "<path>"`, adjusted relative to the generated bundle directory so it resolves to the same archive |
| local package directory `path` | resolve the Legacy package archive path, then use that archive as `source`; see **Local package preparation** |
| `namespace` | package `namespace` |
| `optionalComponents` | `optional_components` |
| `publicKey` | `signature_verification { public_key = file("...") }` when the value is a path; preserve literal key content as an HCL string only when it is clearly intended as content |
| `keylessVerification` | `signature_verification { keyless { ... } }`, changing camelCase keys to the documented snake_case keys |
| static override `values` without Legacy `${NAME}` placeholders | nested YAML at the Zarf-mapped source path for each override target path; see **Override path mapping** |
| static override `values` containing Legacy `${NAME}` placeholders | translate each resolvable scalar placeholder to `{{ .vars.<package>.<normalized_name> }}` at the Zarf-mapped source path, using its collision-safe key when needed; see **Legacy placeholder translation** and **Override path mapping** |
| scalar, non-file override `variables` with a configured value or Legacy default | nested YAML at the Zarf-mapped source path using a type-aware `{{ .vars.<package>.<normalized_name> }}` template and collision-safe key when needed; put defaults/config values under that package in HCL; see **YAML scalar rendering** |
| `options.architecture`, `log_level`, `tmp_dir` | same-name fields in `config.uds.hcl` `options` |
| `options.oci_concurrency` | `options.concurrency` |
| legacy `insecure` | manual decision between `plain_http` and `skip_tls_verify`; do not choose automatically |

Normalize legacy override variable names to lowercase snake case (for example,
`REPLICA_COUNT` becomes `replica_count`) consistently in values files and HCL.
Use package-scoped variables for values-file templates. Only top-level scalar values
are passed through to Zarf package-variable substitutions.

Before generating variables for each package, detect distinct Legacy names that
normalize to the same key. Allocate stable, distinct template keys for every such
collision in Legacy source order by appending `_1`, `_2`, and so on, skipping keys
already used by a non-colliding variable or an earlier allocation. For example,
`fooBar` and `foo_bar` become `foo_bar_1` and `foo_bar_2`. Use each allocated key
consistently in `config.uds.hcl`, values-file templates, and report references, and
record the original-to-generated mapping in the source-attribution table. If a
colliding name is consumed directly by Zarf, or a stable allocation cannot preserve
the required variable semantics, mark it **needs variable-normalization review**
rather than merging the values or choosing a silent fallback.

### Package-scoped template access

Use dot access only when both the package label and normalized variable name are
valid Go-template identifiers. When either key contains a hyphen or another
non-identifier character, use `index` with the exact generated configuration keys:

```text
{{ index (index .vars "package-name-1") "replica_count" }}
```

In particular, repeated-package labels ending in `-1`, `-2`, and so on always
require this form. Do not substitute a separately normalized package key unless the
corresponding `config.uds.hcl` key and every template reference are changed together.

### Direct Zarf package variables

Inspect the supplied package's `zarf.yaml` for direct Zarf package-variable inputs
(for example, `###ZARF_PKG_VAR_NAME###`). A Legacy package-scoped config value used
only by a generated values-file template remains under its package object. A scalar
value consumed directly by Zarf must instead be a collision-free top-level
`variables` entry in `config.uds.hcl`, because Next forwards only top-level scalars
to Zarf's package-variable map.

Before lifting a value, verify that its normalized name, uppercase Zarf name, and
value do not conflict with another direct Zarf variable. Retain the package-scoped
entry as well only when generated values-file templates also need it. If `zarf.yaml`
cannot be inspected, the input is non-scalar, or lifting would change the scope or
collide with another package value, mark it **needs Zarf variable-scope review** in
the migration report; do not silently leave it nested or choose a renamed fallback.

### YAML scalar rendering

Values files are rendered before Next parses them as YAML. For a template that is
the complete value of a known string scalar, render a YAML double-quoted string with
Go template formatting, for example:

```yaml
host: {{ printf "%q" (index (index .vars "package-name-1") "host") }}
```

This preserves strings containing YAML-sensitive content such as `:`, `#`, newlines,
or alias-like prefixes. Render known numeric and boolean scalars unquoted so their
YAML types remain numeric and boolean. For a template embedded in a larger string,
quote the complete rendered scalar with an equivalent explicit template expression;
do not quote only the interpolated portion. If the Legacy type is unknown, a value
cannot be represented safely by this form, or quoting would change its intended YAML
type, mark it **needs YAML scalar rendering review** in the migration report.

When a scalar Legacy override variable has neither a configured value nor a Legacy
default, omit its generated values entry so the chart default remains in effect.
Record it as **preserved unset override** in the migration report. Do not emit an
unconditional `{{ .vars... }}` reference: Next renders values templates with missing
keys as errors. If the user needs an optional Next configuration value instead,
mark that behavior **needs optional-value design** rather than choosing a fallback.

Do not apply direct `{{ .vars... }}` interpolation to a list, object, or a Legacy
chart variable with `type: file`. Legacy sends lists and objects through Helm's JSON
value handling and resolves file variables as file content; rendering those values as
a scalar template can produce Go representations such as `map[...]` or a file path
rather than the intended YAML. Mark each such variable **needs type-aware values
review** in the migration report. Generate an explicit `range`/`with` YAML template
only when the supplied value shape and desired YAML representation are unambiguous;
otherwise require the user to provide the values-file structure or file-content
configuration. Preserve the Legacy variable type and source location in the report.

### Legacy placeholder translation

Legacy expands `${NAME}` placeholders inside static override values before passing
scalars, lists, and objects to Helm. Next values files render Go-template expressions
instead. Scan every static override value recursively for `${NAME}` and, when the
corresponding Legacy variable is a known scalar with an unambiguous package-scoped
Next configuration value, replace it with:

```text
{{ .vars.<package>.<normalized_name> }}
```

Keep the replacement in the same scalar, list item, or object property so the
generated YAML preserves the surrounding value shape. Add the variable to the
package-scoped `config.uds.hcl` values and cite both the static override location and
the variable source in the migration report. Apply **Package-scoped template access**
when either generated key is not a Go-template identifier. Use the collision-safe key
allocated for that Legacy variable, when applicable.

Do not translate a placeholder whose variable is absent, complex, file-backed, or
whose use in a YAML key or mixed-type value makes the resulting YAML ambiguous. Mark
it **needs Legacy placeholder review** and retain the literal source text only in the
report or a clearly labelled comment; do not present a literal `${NAME}` as a working
Next values-file value.

### Override path mapping

Legacy override paths address the Helm chart value target. For each override, inspect
the corresponding component and chart `values` mapping in the supplied `zarf.yaml`.
Zarf extracts package values from `sourcePath` and writes them to `targetPath`; the
generated package values file must therefore contain the value at the mapped source,
not blindly at the Legacy override path.

Match the Legacy override path against the mapping's `targetPath`. When the target
path is an ancestor, append the remaining path segments to its `sourcePath`; for
example, target `.distribution` and source `.registry` map Legacy
`.distribution.host` to generated `.registry.host`. Prefer the longest matching
target-path prefix. Generate the nested YAML at that resolved source path, while
retaining the Legacy component, chart, target path, source path, and mapping rule in
the migration report.

If the component/chart cannot be inspected, no target-path mapping matches, multiple
mappings have the same most-specific match, or a mapping cannot preserve the override
shape, mark the override **needs Zarf source-path mapping review**. Do not generate a
values entry at the Legacy target path unless that is also the verified Zarf source
path.

### Repeated package names

Before generating package blocks, count Legacy package names. Next package labels
must be unique, while Legacy permits repeated names for separate instances. First
reserve every original source package name, including names that are unique and names
such as `api-1` that could otherwise be mistaken for an instance label. Then, for
each repeated Legacy name in source order, allocate the lowest positive suffix whose
`<legacy-name>-<n>` label is neither reserved nor already allocated. For example,
with `api`, `api`, and `api-1`, preserve `api-1` and assign the two `api` instances
`api-2` and `api-3`:

```text
<legacy-name>-<first-available-n>
<legacy-name>-<next-available-n>
```

Use the generated instance label consistently for the Next package block, values-file
path, package-scoped variables, `depends_on` references, and any generated report
references. If a Legacy package-scoped configuration applies to every repeated
instance, duplicate it for each generated instance label; preserve distinct Legacy
overrides with their corresponding instance. Do not use the duplicate Legacy name as
a Next label or emit an invalid bundle with duplicate blocks. Use the `index` form
from **Package-scoped template access** for every values-file reference to an
instance's package-scoped variable.

Add a source-attribution-table row for every renamed instance. State the original
Legacy package name, generated instance label, source-order reason, and any reserved
or previously allocated labels skipped during suffix selection. Retain the original
package name in nearby comments or the report so a reviewer can distinguish the Next
instance identity from the package artifact's own metadata.

Every Next package must declare one verification posture. Preserve a legacy public-key
or keyless configuration. If the legacy package has neither, do not silently disable
verification: emit an unresolved `signature_verification` TODO, using this commented
selection template, and list it as a blocking manual decision:

Never select, uncomment, or replace any option in this template on the user's behalf,
including for a local test. Bundle artifact signing with `--unsigned` does not
authorize `verify = false`; they are independent decisions. Ask the user to choose a
package-verification posture before a validation that requires one.

When materializing the template, use `uds bundle create .` and state that the command
must be run from the generated bundle directory. This keeps the command executable
without a synthetic `<bundle-directory>` placeholder. In the migration report, also
give the equivalent command from the user's current directory using the actual output
directory path (for example, `./.next`).

```hcl
signature_verification {
  # Choose exactly one option. Uncomment only the line(s) explicitly identified below; leave all other comments unchanged.
  # Package verification and bundle-artifact signing are independent decisions.

  # Option 1: key-based verification. Uncomment only the following line to select this option.
  # public_key = file("keys/<package>.pub")
  # From this bundle directory, sign the created artifact with a private key or KMS URI:
  # CLI_FEATURES=NextMode=true uds bundle create . --signing-key <private-key-or-kms-uri>

  # Option 2: keyless verification. Uncomment the following four lines to select this option.
  # keyless {
  #   certificate_identity_regexp = "https://..."
  #   certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
  # }
  # From this bundle directory, sign the created artifact with an OIDC identity:
  # CLI_FEATURES=NextMode=true uds bundle create . --keyless

  # Option 3: local-alpha only; disables package verification. Uncomment only the following line to select this option.
  # verify = false
  # From this bundle directory, create an unsigned artifact:
  # CLI_FEATURES=NextMode=true uds bundle create . --unsigned
}
```

State that the options are mutually exclusive: enabled verification requires exactly
one `public_key` or `keyless` configuration, and keyless verification requires one
certificate identity constraint and one OIDC issuer constraint. The all-commented
template intentionally fails create-time validation until the user chooses a trust
posture. `verify = false` is an explicitly labelled, security-reducing local-alpha
option only. Independently, every `uds bundle create` invocation must select exactly
one artifact-signing mode: `--signing-key <private-key-or-kms-uri>`, `--keyless`, or
`--unsigned`; the commands beside the template options are common companion choices,
not required verification-to-signing pairings.

### Local package preparation

A local Legacy path is not automatically a usable Next source just because it
contains `zarf.yaml`. Next loads local packages through Zarf's canonical package
layout, which requires a generated `checksums.txt` in a directory source; a generated
`.tar.zst` archive is also accepted. Do not claim that a bare authoring directory was
validated.

Legacy resolves a directory path relative to the Legacy bundle manifest, then expects
a generated archive in that directory. Derive the archive name from the effective
Legacy architecture, package name, `ref`, and optional `flavor`:

```text
zarf-package-<name>-<architecture>-<ref>[-<flavor>].tar.zst
```

For a package named `init`, Legacy uses:

```text
zarf-init-<architecture>-<ref>[-<flavor>].tar.zst
```

The effective Legacy architecture follows its precedence: explicit CLI architecture,
bundle metadata architecture, bundle build architecture, then the runtime host
architecture. When the supplied inputs do not establish that value, do not guess or
emit a directory source. Mark the package **needs local package preparation** and
report the directory plus the archive-name template that requires the user's target
architecture. When the architecture is known, emit the resolved archive path as the
canonical Next `source`, even when the archive is not present; mark the missing
artifact as blocking manual work. The legacy `ref` has no separate Next field, but is
preserved as part of the derived archive filename and must be cited in the report.

Include the exact resolved archive path and an executable preparation command in the
migration report. Do not create the package or replace the canonical migrated source
unless the user explicitly asks for a separate local validation copy. A validation
copy must be clearly labelled as non-equivalent and must leave the canonical migrated
files unchanged.

For an explicitly authorized validation copy, use an output directory outside the
canonical migration directory so the Legacy input and migration output remain
untouched, for example:

```sh
mkdir -p .next-validation/packages
CLI_FEATURES=NextMode=true uds tools zarf package create <legacy-local-package-path> --output .next-validation/packages --confirm
```

After the command produces an archive, update only the validation copy's
corresponding package `source` to the actual archive filename, such as
`packages/zarf-package-<name>-<architecture>-<version>.tar.zst`. Do not invent the
architecture or generated filename. Record the source replacement and its
non-equivalence in the validation copy's report. Never replace a Legacy OCI
`repository`/`ref` source with a local package, registry, or fixture automatically;
an explicitly authorized validation substitution must be isolated and reported as
non-equivalent. Package creation does not itself require a cluster, although the
package's own build inputs can require network access or other prerequisites.

## Override review

Legacy overrides target a component and chart; Next values files are package-level.
For each override, retain the Legacy component/chart location, original target path,
and resolved Zarf source path in the migration report. Check that the target
package's `zarf.yaml` maps every generated values-file entry from that source path to
the intended chart target path. If `zarf.yaml` is absent, a target-path mapping is
missing or ambiguous, two charts write conflicting paths, a Legacy `valuesFiles`
path cannot be inspected, or an override sets a chart-specific namespace, generate
the proposed file only when its content is unambiguous and mark it **needs Zarf
source-path mapping review**. Never say it is equivalent until that review passes.

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

Recommend a non-production development deployment before creating and signing the
artifact. The report must name the actual generated output directory rather than
relying on the current directory, for example:

```sh
CLI_FEATURES=NextMode=true uds bundle dev deploy <output-dir> --config <output-dir>/config.uds.hcl
```

Append the `--config` argument whenever the migration generated `config.uds.hcl`;
omit it only when that file was not generated. Next artifacts use `.tar.zst`; source
definitions and artifacts are not backward compatible. Point the user to
`docs/how-to-guides/migrate-legacy-to-next.mdx` in this repository (or the published
Migration guide) for the maintained human walkthrough.

Before declaring the canonical migration ready for review, reconcile its report with
every user-confirmed manual edit while preserving every unresolved blocker the user
has not chosen to resolve. Do not record a validation-copy source substitution or a
test-only `verify = false` selection as a conversion of the canonical migration. A
deliberately selected `verify = false` remains explicitly labelled as local-alpha and
security-reducing in the validation copy's report.
