---
name: migrate-legacy-bundle-to-next
description: Convert a Legacy UDS CLI uds-bundle.yaml into reviewed UDS CLI Next HCL files. Use when migrating a bundle authoring workflow; do not use for arbitrary YAML-to-HCL conversion.
---

# Migrate a Legacy bundle to UDS CLI Next

Produce a reviewable first-pass migration from `uds-bundle.yaml`. Keep the source file
unchanged. The deliverable is not complete until it calls out every source construct
that has no safe Next equivalent.

This AI-assisted workflow is experimental. It produces a proposed migration, not a
compatibility guarantee. Begin the migration report with an **Experimental migration
warning** that requires review of generated files, package sources, variable behavior,
Zarf mappings, and verification settings before use in any environment. State that a
successful artifact build does not prove deployment equivalence.

## Inputs and output

Ask for the legacy bundle contents or path.

For a new migration, check whether the requested output directory already exists
before writing any output. If it does, stop without reading, merging, changing, or
deleting anything in that directory. Ask the user to remove or move the existing
directory, then wait for confirmation that the requested output directory is absent.
Do not select a different output directory or overwrite a prior migration
automatically.

This guard does not apply when the user explicitly asks to continue, correct, or
reconcile a named active migration output after review or validation. In that case,
inspect the existing generated files and migration report, then change only the
generated Next files needed for the requested correction. Do not delete or wholesale
replace the output directory, merge an unrelated prior migration, or modify Legacy
inputs. If the user does not explicitly authorize a continuation, treat an existing
directory as a new-migration conflict.

For every local package `path`, inspect the target when it is available. Determine
whether it is a canonical Zarf package source: either a `.tar.zst` archive or a
package directory that includes the generated `checksums.txt`. A directory containing
only authoring inputs such as `zarf.yaml` needs package preparation before Next can
create a bundle from it.

For every package with Legacy `overrides`, discover its Zarf values mappings before
asking the user to supply `zarf.yaml`. Read the definition from an available local
package layout or, for an OCI package, inspect its definition with the read-only
command:

```sh
CLI_FEATURES=NextMode=true uds tools zarf package inspect definition 'oci://<repository>:<effective-ref>'
```

This may use the user's existing registry credentials but must not write to a registry
or perform a cluster operation. Respect an explicit instruction not to run UDS
commands or access the network. If the package cannot be inspected because access is
unavailable or prohibited, record its mappings as unverified and ask for the relevant
`zarf.yaml` only when it is needed to resolve an override; do not make copy/paste the
normal discovery path. Apply **Sensitive values** to inspected content.

## Sensitive values

Treat a value as sensitive when disclosing it could grant access, enable
impersonation or decryption, or expose protected data. This includes passwords, API
and bearer tokens, registry or cloud credentials, kubeconfigs, private keys,
client-key material, secret-manager credentials, and connection strings containing
credentials. Also honor any user or organization classification. Package references,
namespaces, public keys, keyless certificate identities, and public Sigstore trusted
roots are not sensitive by default.

When a source contains a sensitive value, never echo it in chat, report prose, or a
generated-file comment. Report only its source location and **needs sensitive-value
handling**. Stop before copying it into generated output and ask the user to choose
one of these paths: explicitly authorize a local-only copy into a specified untracked
file, or generate a redacted migration that requires manual secret injection. Preserve
the Legacy input in either case. Do not silently substitute a
placeholder, claim a redacted result is equivalent, or choose a secret-storage
mechanism on the user's behalf.

Write all of the following under the requested output directory:

1. `bundle.uds.hcl`, with `uds { bundle_api_version = "uds.dev/v1alpha1" }`, metadata,
   package blocks, and package verification posture.
2. A package-level `values/<package>.yaml` for each safely transcribed legacy override,
   plus the corresponding `values_files` entry. Preserve effective Legacy Helm value
   types and render legacy override variables as `{{ .vars.<package>.<variable> }}`.
3. `defaults.uds.hcl` for every safely representable Legacy override variable default
   that belongs to this migrated bundle. It may contain only `variables`, never
   `options`. Do not translate deployment-time values or settings.
4. A migration report listing converted fields, manual work, unsupported features,
   and the exact Next commands to use.

Create required subdirectories and files directly; do not use fenced response blocks
as a substitute for writing the files. In the final response, list the generated paths
and unresolved blockers without duplicating the file contents. Do not claim that
generated files were validated or that an override will work unless the required Zarf
mapping was obtained or supplied and checked.

## Source attribution

Make every migration reviewable against its Legacy input. Count source lines from the
input file exactly as provided and use one-based `path:line` or `path:start-end`
references. Do not guess a location when an input is incomplete or generated; say so
in the migration report instead.

Add concise comments immediately before generated semantic blocks, not every
attribute. Attribute each `metadata`, `package`, `signature_verification`, and package
values section that came from a distinct Legacy source range. For example:

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
| unique package `name` valid in Next | `package "<name>"` label |
| package `name` containing `/` or `\\`, or equal to `.` or `..` | collision-free identifier-safe Next label; see **Next package-label validation** |
| repeated package `name` | unique collision-free instance labels, allocated in Legacy source order; see **Repeated package names** |
| `repository` plus `ref` | `source = "oci://<repository>:<ref>"` |
| local package `path` ending in `.tar.zst` | `source = "<path>"`, adjusted relative to the generated bundle directory so it resolves to the same archive |
| local package directory `path` | resolve the Legacy package archive path, then use that archive as `source`; see **Local package preparation** |
| `namespace` | package `namespace` |
| `optionalComponents` | `optional_components`, after removing exact duplicate entries while preserving first-occurrence order |
| `publicKey` | `signature_verification { public_key = ... }` with the Legacy key content encoded as an HCL string or heredoc; Legacy treats this field as content, not a path; see **HCL-safe strings** |
| `keylessVerification` | `signature_verification { keyless { ... } }`, changing camelCase keys to the documented snake_case keys; see **HCL-safe strings** |
| static override `values` without Legacy `${NAME}` placeholders | nested YAML with the effective Legacy Helm scalar type at the Zarf-mapped source path for each override target path; see **Legacy Helm scalar coercion**, **Literal Go-template delimiters**, and **Override path mapping** |
| static override `values` containing Legacy `${NAME}` placeholders | **not converted**; report for manual review |
| scalar, non-file override `variables` with a non-null Legacy default | nested YAML at the Zarf-mapped source path using a type-aware `{{ .vars.<package>.<normalized_name> }}` template and collision-safe key when needed; write the default to `defaults.uds.hcl`; see **Legacy Helm scalar coercion** and **YAML scalar rendering** |
| scalar override `variables` with an effective Legacy null default | **not converted**; Next configuration variables do not accept null; see **Legacy Helm scalar coercion** |
| Legacy configured override values, `options`, or `insecure` | **not converted**; report for manual handling |

For each package, stable-deduplicate Legacy `optionalComponents` before emitting
`optional_components`: retain the first occurrence of each exact component name and
remove later identical entries. Record removed duplicates in the migration report so
the Next output remains reviewable while preserving Legacy's effective component
selection. If an entry cannot be read as a component name, mark that package **needs
optional-components review** rather than emitting invalid HCL.

### HCL-safe strings

Encode every migrated literal string as HCL before emitting it, including metadata,
package sources, and verification fields. In quoted
strings, escape HCL-special characters and replace literal `${` with `$${` and `%{`
with `%%{`; do this in addition to normal backslash and quote escaping. Preserve
intentional HCL expressions such as `file("...")` as expressions rather than quoting
them. For multiline content, use a heredoc such as `<<-MIGRATED_VALUE` only when its
delimiter does not occur on a line by itself in the value; otherwise choose a unique
delimiter. Apply the same `$${` and `%%{` escapes inside the heredoc.

HCL quoted strings interpret backslashes. Escape each literal regex backslash when
emitting `certificate_identity_regexp` or `certificate_oidc_issuer_regexp`; for
example, Legacy `https://github\.com/...` becomes
`"https://github\\.com/..."` in HCL. Preserve the intended regular expression, not
the raw YAML spelling.

Legacy `publicKey` and `keylessVerification.trustedRoot` values are inline content:
Legacy writes their literal strings to temporary verification files. Preserve that
content as an HCL string or, for multiline PEM keys, TrustedRoot JSON, and other
multiline values, an HCL heredoc. Do not interpret a Legacy value as a file path or
invent one. A `file("...")` expression is appropriate only when the user separately
provides and authorizes a generated-output file containing the same material. Record
the representation and source location in the migration report.

### Literal Go-template delimiters

Next parses generated values files with Go `text/template` only when the effective
bundle defaults supply a non-nil variables map. Determine that map before changing
static values. When it is nil, preserve literal `{{` and `}}` unchanged. Only when it
is known to be non-nil,
scan every static Legacy override scalar, list element, and object value for those
delimiters and emit `{{"{{"}}` and `{{"}}"}}`, respectively, so rendering produces the
original delimiter rather than evaluating it. Do not escape intentional Next
expressions generated by this migration, such as `{{ .vars... }}` or `{{ index ... }}`.
If the effective variables map is unknown, it is unclear whether a source delimiter
is literal or intended for Next rendering, or escaping cannot preserve the YAML value
shape, mark it **needs literal-template review** instead of classifying the override
as safely transcribed.

Normalize Legacy override variable names to lowercase snake case (for example,
`REPLICA_COUNT` becomes `replica_count`) for package-scoped values-file templates.

Before snake-case allocation, group each package's Legacy override variable names by
their uppercase identity. Names such as `foo` and `FOO` are aliases in Legacy and
must not receive independent Next inputs. Map aliases to one shared Next key only if
their defaults are all absent or semantically identical. If aliases have differing
defaults, a non-scalar or file value, or another ambiguous source, mark the group
**needs case-alias-variable review** rather than splitting or silently choosing a value.
Record every Legacy alias and its shared key or review disposition in the
source-attribution table.

After resolving uppercase aliases, detect distinct remaining Legacy identities that
normalize to the same key. Allocate stable, distinct template keys for every such
collision in Legacy source order by appending `_1`, `_2`, and so on, skipping keys
already used by a non-colliding variable or an earlier allocation. For example,
`fooBar` and `foo_bar` become `foo_bar_1` and `foo_bar_2`. Use each allocated key
consistently in `defaults.uds.hcl`, values-file templates, and report references, and
record the original-to-generated mapping in the source-attribution table. If a stable
allocation cannot preserve the required variable semantics, mark it **needs
variable-normalization review** rather than merging the values or choosing a silent
fallback.

### Package-scoped template access

Use dot access only when both the package label and normalized variable name are
valid Go-template identifiers. When either key contains a hyphen or another
non-identifier character, use `index` with the exact generated variable keys:

```text
{{ index (index .vars "package-name-1") "replica_count" }}
```

In particular, repeated-package labels ending in `-1`, `-2`, and so on always
require this form. Do not substitute a separately normalized package key unless the
corresponding `defaults.uds.hcl` key and every template reference are changed together.

### Legacy Helm scalar coercion

Legacy sends scalar overrides to Helm as `path=value` and Helm applies its `strvals`
coercion after Legacy YAML parsing and placeholder substitution. Before writing a
static scalar or a scalar override variable/default to Next HCL or YAML, derive that
effective Helm type rather than preserving YAML quotation alone. For example, a Legacy
quoted string `"true"`, `"false"`, `"null"`, or `"42"` becomes the boolean, null, or
integer Helm value. Emit static effective nulls as YAML `null`; they are valid static
Helm values. Do not write an effective null into `defaults.uds.hcl` or another Next
configuration variable: Next rejects null configuration values. Mark a configurable
null **needs null-variable representation review** and block that override until a
verified equivalent is selected.

Apply this conversion before **YAML scalar rendering**. Preserve a string only when
the equivalent Legacy `strvals` input remains a string. If escaping, an unsupported
scalar form, a template result, or an uncertain `strvals` outcome prevents an exact
derivation, mark the override **needs Helm scalar-coercion review** rather than
assuming the source YAML type is equivalent.

Next converts numeric HCL variables to `float64` before rendering values files. For
an effective Legacy integer outside the universally exact `float64` range from
`-2^53` through `2^53`, do not place it in `defaults.uds.hcl` as an HCL number and do
not claim the numeric type is retained. Mark it **needs
large-integer review** unless a verified representation preserves the exact integer
and the intended YAML numeric type.

### YAML scalar rendering

Values files are rendered before Next parses them as YAML. After applying **Legacy
Helm scalar coercion**, for a template that is the complete value of a known string
scalar, render a YAML double-quoted string with Go template formatting, for example:

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

When a scalar Legacy override variable has no Legacy default, omit its generated
values entry so the chart default remains in effect.
Record it as **preserved unset override** in the migration report. Do not emit an
unconditional `{{ .vars... }}` reference: Next renders values templates with missing
keys as errors. Mark an optional value as **not converted** rather than choosing a
fallback.

For every safely representable, non-null Legacy override default, write the mapped
variable and its default value to `defaults.uds.hcl` so the created artifact retains
Legacy fallback behavior. Do not omit or relocate a Legacy default merely because the
user has not separately identified it as portable; require an explicit user decision
to change that behavior. For an effective null default, keep any static null at its
YAML path but do not generate its variable/template or HCL default; record **needs
null-variable representation review** as a blocking item. Record each default and
its source in the migration report. Apply sensitive-value handling before writing a
sensitive default.

Do not apply direct `{{ .vars... }}` interpolation to a list, object, or a Legacy
chart variable with `type: file`. Legacy sends lists and objects through Helm's JSON
value handling and resolves file variables as file content; rendering those values as
a scalar template can produce Go representations such as `map[...]` or a file path
rather than the intended YAML. Mark each such variable **needs type-aware values
review** in the migration report. Generate an explicit `range`/`with` YAML template
only when the supplied value shape and desired YAML representation are unambiguous;
otherwise require the user to provide the values-file structure or file-content input.
Preserve the Legacy variable type and source location in the report.

### Override path mapping

Legacy override paths address the Helm chart value target. For each override, inspect
the corresponding component and chart `values` mapping in the supplied `zarf.yaml`.
Zarf extracts package values from `sourcePath` and writes them to `targetPath`; the
generated package values file must therefore contain the value at the mapped source,
not blindly at the Legacy override path.

Match the Legacy override path against each mapping's `targetPath`. When the target
path is an ancestor, replace that prefix with `sourcePath`; for example, target
`.distribution` and source `.registry` map Legacy `.distribution.host` to generated
`.registry.host`. Prefer the longest matching target-path prefix. A Legacy path that
matches only `sourcePath` is not equivalent: Zarf will write that value to the
different `targetPath`, changing the Helm key. Generate the nested YAML at the
resolved source path only after a target-path match, while retaining the Legacy
component, chart, target path, source path, and mapping rule in the migration report.
Inspect the selected mapping's `excludePaths` before classifying it as usable. If an
excluded path equals the resolved generated source path or is its ancestor, Zarf
deletes the migrated value before extracting `sourcePath`; mark the override **needs
Zarf source-path mapping review** and do not emit it at that path. A target-path match
alone is not sufficient when its migrated source value is excluded.

Before emitting a resolved source path, inspect the `values` mappings for every chart
selected in the package, not only the Legacy override's component/chart. A package
values file is shared across those charts. Treat a mapping whose `sourcePath` equals,
is an ancestor of, or is contained by the emitted source path as an overlapping
consumer. If another chart can consume that value, do not assume the fan-out is
equivalent: record every consumer and its target path, and mark the override **needs
cross-chart values mapping review** unless the supplied Legacy inputs establish the
same override for every consumer with equivalent target behavior. Do not emit an
automatic value for an ambiguous cross-chart mapping.

If the component/chart cannot be inspected, no target-path mapping matches, a
source-only mapping would redirect the Helm key, multiple mappings have the same
most-specific match, a mapped source value is excluded, or a mapping cannot preserve
the override shape, mark the override **needs Zarf source-path mapping review**. Do
not generate a values entry at the Legacy target path unless that is also the verified
Zarf source path.

### Next package-label validation

Before emitting package blocks, validate every Legacy name as a Next package label.
Next rejects names containing `/` or `\\` and the exact names `.` and `..`. For each
such name, allocate a collision-free identifier-safe label in Legacy source order:
start with `package_`, replace every slash or backslash with `_`, map `.` to
`package_current` and `..` to `package_parent`, then append `_1`, `_2`, and so on as
needed to avoid every valid original label and earlier allocation. For example,
`team/api` becomes `package_team_api` unless that label is already reserved. Do not
use the Legacy name as a generated directory, values-file path, variable key, or
dependency reference. Preserve it for package-source semantics, such as the resolved
Legacy archive filename, and record the original-to-generated label mapping in the
migration report. If a stable collision-free label cannot be allocated, mark the
package **needs package-label review** rather than emitting an invalid bundle.

### Repeated package names

Before generating package blocks, count Legacy package names. Next package labels
must be unique, while Legacy permits repeated names for separate instances. First
allocate labels for invalid names under **Next package-label validation**, then reserve
every remaining valid original source name and every allocated label, including names
such as `api-1` that could otherwise be mistaken for an instance label. For each
remaining repeated valid Legacy name in source order, allocate the lowest positive
suffix whose `<legacy-name>-<n>` label is neither reserved nor already allocated. For
example, with `api`, `api`, and `api-1`, preserve `api-1` and assign the two `api`
instances `api-2` and `api-3`:

```text
<legacy-name>-<first-available-n>
<legacy-name>-<next-available-n>
```

Use the generated instance label consistently for the Next package block, values-file
path, package-scoped variables, and any generated report references. If a Legacy
package-scoped default applies to every repeated instance, duplicate it for each
generated instance label; preserve distinct Legacy overrides with their
corresponding instance. Do not use the duplicate Legacy name as a Next label or emit
an invalid bundle with duplicate blocks. Use the `index` form from
**Package-scoped template access** for every values-file reference to an instance's
package-scoped variable.

### Dependency-safe package labels

Next accepts `depends_on` references only in `package.<identifier>` form. A label
may contain a hyphen after its first character, so `package.foo-1` is valid. Preserve
such labels in package blocks and dependency references; `index` remains necessary
only for values-file template access to a package-scoped variable.

When a migration must generate or preserve a dependency involving a label that does
not parse as an HCL identifier, allocate a collision-free identifier-safe package
label instead. Reserve all original and allocated labels, propagate that replacement
to the package block, values-file path, package-scoped variables, templates, and
every dependency reference, and record it in the migration report. If the Legacy
input cannot identify which repeated instance is the dependency target, or relabelling
would be ambiguous, mark it **needs repeated-package dependency review** rather than
guessing.

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
without a synthetic `<bundle-directory>` placeholder. When the effective Legacy
architecture is known, append
`--architecture <effective-legacy-architecture>`; otherwise report architecture as a
manual decision. In the migration report, also give the equivalent command
from the user's current directory using the actual output directory path (for example,
`./.next`) and the same architecture rule.

Keyless verification constraints are not a signing-service profile. Legacy package
fields do not identify Fulcio, signing OIDC, Rekor, or TSA endpoints, and a
`trustedRoot` does not safely determine them. Do not infer those settings. For a
private keyless artifact-signing profile, require user-provided `--fulcio-url`,
`--oidc-issuer`, and, when Rekor is used, `--rekor-url` values. `uds bundle create`
does not expose `--tlog-upload`; if the user requires timestamp-only bundle signing
without Rekor, report that workflow as **needs artifact keyless-signing review**
rather than presenting the `--keyless` example as equivalent.

```hcl
signature_verification {
  # Choose exactly one option. Uncomment only the line(s) explicitly identified below; leave all other comments unchanged.
  # Package verification and bundle-artifact signing are independent decisions.

  # Option 1: key-based verification. Uncomment only the following line to select this option.
  # public_key = file("keys/<package>.pub")
  # From this bundle directory, sign the created artifact with a private key or KMS URI:
  # CLI_FEATURES=NextMode=true uds bundle create . --architecture <effective-legacy-architecture> --signing-key <private-key-or-kms-uri>

  # Option 2: keyless verification. Uncomment the following four lines to select this option.
  # keyless {
  #   certificate_identity_regexp = "https://..."
  #   certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
  # }
  # From this bundle directory, sign the created artifact with an OIDC identity:
  # CLI_FEATURES=NextMode=true uds bundle create . --architecture <effective-legacy-architecture> --keyless

  # Option 3: local-alpha only; disables package verification. Uncomment only the following line to select this option.
  # verify = false
  # From this bundle directory, create an unsigned artifact:
  # CLI_FEATURES=NextMode=true uds bundle create . --architecture <effective-legacy-architecture> --unsigned
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

Encode every substituted command value as one POSIX-shell argument before placing it
in a generated `sh` block. This includes every Legacy-controlled path, ref, flavor,
and architecture, plus user-provided signing inputs. Use single quotes, replacing an
embedded apostrophe with `'"'"'`; quote simple-looking values too. Never insert a raw
value from a manifest, configuration, or user input into shell syntax. In the
templates below, each angle-bracketed value represents one such shell-quoted argument.

For an explicitly authorized validation copy, use an output directory outside the
canonical migration directory so the Legacy input and migration output remain
untouched. When the effective Legacy architecture is known, pass it to Zarf so the
prepared archive has the same architecture as the canonical migrated package; for
example. When the Legacy package entry specifies a flavor, also pass that exact
flavor with `--flavor <legacy-flavor>`; otherwise omit the flag:

```sh
mkdir -p .next-validation/packages
CLI_FEATURES=NextMode=true uds tools zarf package create '<legacy-local-package-path>' --architecture '<effective-legacy-architecture>' --flavor '<legacy-flavor>' --output .next-validation/packages --confirm
```

When the canonical migration retains `public_key` verification, require the user to
provide a private key or KMS URI compatible with that public key, then append
`--signing-key <package-private-key-or-kms-uri>` to the preparation command. Never
copy signing material into the migration report or generated files. When it retains
keyless verification, create the package first, then sign its actual archive with an
identity that satisfies the retained certificate and issuer constraints. Before
giving this command, require the user to provide the signing-service profile when it
is not the public Sigstore default: `--fulcio-url`, `--oidc-issuer`, and, when Rekor
is used, `--rekor-url`. Do not derive those endpoints from `trustedRoot` or from a
certificate-issuer verification constraint. Record unavailable endpoint or identity
inputs as **needs keyless signing-profile review** and block validation rather than
silently contacting public services. Append every user-provided signing-profile flag
to this command:

```sh
mkdir -p .next-validation/signed-packages
CLI_FEATURES=NextMode=true uds tools zarf package sign '.next-validation/packages/<unsigned-archive>.tar.zst' --keyless --architecture '<effective-legacy-architecture>' --output .next-validation/signed-packages --confirm
```

When the retained Legacy `keylessVerification.useSignedTimestamps` is `true`, require
a suitable `--tsa-server-url <rfc3161-timestamp-authority-url>` and append it to the
command. Otherwise omit that flag. Ask whether the retained profile is timestamp-only
because Rekor is unavailable; if so, also append `--tlog-upload=false` so Zarf does
not auto-enable Rekor upload for `--keyless`. Otherwise retain the user's confirmed
Rekor decision. If the required TSA, private endpoint, or identity input is
unavailable, mark validation as blocked rather than producing a signature that cannot
satisfy the retained verification policy.

If compatible signing material is unavailable, do not change the canonical migration.
Only when the user explicitly selects it, change `verify = false` in the validation
copy as a local-alpha, security-reducing test adaptation and record its
non-equivalence in that copy's report. Otherwise mark validation as blocked by the
retained package-verification policy.

After the selected path produces its final archive, update only the validation copy's
corresponding package `source` to the actual archive filename, such as
`packages/zarf-package-<name>-<architecture>-<version>.tar.zst` or
`signed-packages/<actual-signed-archive>.tar.zst`. Do not invent the architecture or
generated filename. Record the source replacement and its non-equivalence in the
validation copy's report. Never replace a Legacy OCI `repository`/`ref` source with a
local package, registry, or fixture automatically; an explicitly authorized
validation substitution must be isolated and reported as non-equivalent. Package
creation does not itself require a cluster, although the package's own build inputs
can require network access or other prerequisites.

## Legacy values-files precedence

For every component/chart override with `valuesFiles`, inspect every referenced file
and preserve the Legacy merge result before generating a Next package values file.
Resolve an absolute entry as written and every relative entry against the Legacy
bundle's source directory, not the agent's current working directory or the generated
Next directory. Use and record the resolved Legacy path before reading or merging it.
Process files in their declared list order. Legacy treats every file as one value per
top-level key: when a later file repeats a top-level key, replace the entire earlier
value at that key rather than deep-merging nested mappings. Then apply inline Legacy
`values` entries last; they have highest precedence for matching value paths. Generate
the Next values entry only from that final result, preserving YAML value types, and
record the ordered source files and any replaced keys or paths in the migration
report.

If a values file cannot be read, its YAML cannot be merged without changing a value
shape, the chart mapping cannot be verified, or the original precedence is ambiguous,
mark the override **needs Legacy values-files merge review**. Do not report an
arbitrary fold or a reordered list as converted.

## Override review

Legacy overrides target a component and chart; Next values files are package-level.
For each override, retain the Legacy component/chart location, original target path,
and resolved Zarf source path in the migration report. Check that the target
package's inspected or supplied Zarf definition maps every generated values-file entry
from that source path to the intended chart target path. If the definition is
unavailable, a target-path mapping is missing or ambiguous, two charts write
conflicting paths, a Legacy `valuesFiles` path cannot be inspected, or an override
sets a chart-specific namespace, generate the proposed file only when its content is
unambiguous and mark it **needs Zarf source-path mapping review**. Never say it is
equivalent until that review passes.

## Always report these gaps

Flag rather than drop any occurrence of:

- bundle `kind` and `build` metadata;
- metadata `architecture` (record as a manual architecture decision), `uncompressed`, URL,
  authors, documentation, source, vendor, or aggregate checksum;
- package `description`, `timeout`, `flavor`, `imports`, and `exports`;
- legacy `valuesFiles` that cannot be folded with their Legacy precedence into a
  package values file and verified against Zarf mappings;
- an override variable or default whose effective Legacy value is null; verify a Next
  representation before converting it, because null is not accepted in Next
  configuration variables;
- `shared` values, `uds_cache`, `retries`, `UDS_<NAME>` environment variables, and
  `--set` workflows;
- legacy command/flag behavior without a Next equivalent, including `uds logs`,
  `uds list`, deploy `--resume`, `--retries`, `--force-conflicts`, and inspect
  `--sbom`, `--list-images`, and `--list-variables`.

`imports` and `exports` have no direct Next equivalent. Report them as not converted;
do not translate them to `depends_on` unless the user independently establishes an
ordering dependency.

## Package deployment order

Legacy deploys packages sequentially in source order. Next deploys independent
packages concurrently, so source-list position does not preserve a required order.
Review every ordering-sensitive relationship before generating a deployment command,
including an init package before standard packages that require an initialized
cluster, explicit user-provided ordering, and package-manifest requirements.

When the required relationship is established and its source and target packages are
unambiguous, emit an identifier-safe `depends_on` edge on the dependent package; for
example, `depends_on = [package.init]`. Use **Dependency-safe package labels** when
either package label cannot be referenced as `package.<identifier>`. Do not serialize
the whole Legacy list merely because it was sequential.

If required ordering cannot be established from the supplied inputs, record **needs
package-ordering review** as a blocking migration-report item. Do not recommend a
source-based or artifact deployment until the user either confirms the required
dependencies or establishes that the cluster is already initialized and no package
ordering is required.

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

For verification-sensitive validation, prefer deploying the exact artifact produced
by a successful `uds bundle create`. Record its exact local path (or immutable OCI
reference after publication) in the report and use `uds bundle deploy` so artifact
integrity verification occurs before package deployment. Do not recommend deploying
an unsigned artifact unless the user explicitly authorizes the local-alpha,
security-reducing `--skip-signature-verification` bypass; otherwise require a signed
artifact and its appropriate verification inputs.

```sh
CLI_FEATURES=NextMode=true uds bundle deploy '<created-artifact>.tar.zst'
```

Treat source `dev deploy` as a separate, non-production development workflow, not as
validation of the created artifact. It reloads package sources, so a mutable OCI tag
can resolve to bytes different from those verified during `bundle create`, and it can
continue after package-signature failures. Do not recommend it while a
package-verification TODO is unresolved or after a failed verification. When the user
explicitly selected `verify = false` for a validation copy, label its source
deployment as an unverified, local-alpha, security-reducing workflow. Source `dev
deploy` discovers an adjacent `defaults.uds.hcl`. The report must name the actual
generated output directory rather than relying on the current directory, for example:

```sh
CLI_FEATURES=NextMode=true uds bundle dev deploy <output-dir> --architecture <effective-legacy-architecture>
```

The migration does not cover deployment-time settings. When the effective Legacy
architecture is known, append
`--architecture <effective-legacy-architecture>`; otherwise record that selection as
manual work. Next artifacts use `.tar.zst`; source definitions and artifacts are not
backward compatible. Point the user to
`docs/how-to-guides/migrate-legacy-to-next.mdx` in this repository (or the published
Migration guide) for the maintained human walkthrough.

Before declaring the canonical migration ready for review, reconcile its report with
every user-confirmed manual edit while preserving every unresolved blocker the user
has not chosen to resolve. Before `uds bundle create`, check that generated
`defaults.uds.hcl` and other Next configuration variables contain no null values;
static YAML nulls remain valid and must not be changed solely for that check. Do not
record a validation-copy source substitution or a test-only `verify = false` selection
as a conversion of the canonical migration. A deliberately selected `verify = false`
remains explicitly labelled as local-alpha and security-reducing in the validation
copy's report.
