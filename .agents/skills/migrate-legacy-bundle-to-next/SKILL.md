---
name: migrate-legacy-bundle-to-next
description: Convert a Legacy UDS CLI uds-bundle.yaml and optional uds-config.yaml into reviewed UDS CLI Next HCL files. Use when migrating a bundle authoring workflow; do not use for arbitrary YAML-to-HCL conversion.
---

# Migrate a Legacy bundle to UDS CLI Next

Produce a reviewable first-pass migration from `uds-bundle.yaml` and, when supplied,
`uds-config.yaml`. Keep the source files unchanged. The deliverable is not complete
until it calls out every source construct that has no safe Next equivalent.

This AI-assisted workflow is experimental. It produces a proposed migration, not a
compatibility guarantee. Begin the migration report with an **Experimental migration
warning** that requires review of generated files, package sources, variable behavior,
Zarf mappings, and verification settings before use in any environment. State that a
successful artifact build does not prove deployment equivalence.

## Inputs and output

Ask for the legacy bundle and optional config contents or paths. If the bundle uses
`overrides`, also ask for each referenced package's `zarf.yaml` when it is available;
the package mappings determine whether a generated values file is usable. Also ask
whether the Legacy workflow used `uds dev deploy --ref <package>=<ref>` and, for each
such remote package, its effective resolved ref including the platform-specific digest
when Legacy resolved one.

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
configuration file, or generate a redacted migration that requires manual secret
injection. Preserve the Legacy input in either case. Do not silently substitute a
placeholder, claim a redacted result is equivalent, or choose a secret-storage
mechanism on the user's behalf.

Return all of the following:

1. `bundle.uds.hcl`, with `uds { bundle_api_version = "uds.dev/v1alpha1" }`, metadata,
   package blocks, and package verification posture.
2. A package-level `values/<package>.yaml` for each safely transcribed legacy override,
   plus the corresponding `values_files` entry. Preserve effective Legacy Helm value
   types and render legacy override variables as `{{ .vars.<package>.<variable> }}`.
3. `config.uds.hcl` for explicitly configured deploy-time variables and options.
   Generate `defaults.uds.hcl` for every safely representable Legacy override
   variable default, plus any user-identified portable build-time defaults; it may
   contain only `variables`, never `options`.
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
| unique package `name` valid in Next | `package "<name>"` label |
| package `name` containing `/` or `\\`, or equal to `.` or `..` | collision-free identifier-safe Next label; see **Next package-label validation** |
| repeated package `name` | unique collision-free instance labels, allocated in Legacy source order; see **Repeated package names** |
| `repository` plus effective `ref` | `source = "oci://<repository>:<ref>"`; see **Development ref overrides** |
| local package `path` ending in `.tar.zst` | `source = "<path>"`, adjusted relative to the generated bundle directory so it resolves to the same archive |
| local package directory `path` | resolve the Legacy package archive path, then use that archive as `source`; see **Local package preparation** |
| `namespace` | package `namespace` |
| `optionalComponents` | `optional_components`, after removing exact duplicate entries while preserving first-occurrence order |
| `publicKey` | `signature_verification { public_key = ... }` with the Legacy key content encoded as an HCL string or heredoc; Legacy treats this field as content, not a path; see **HCL-safe strings** |
| `keylessVerification` | `signature_verification { keyless { ... } }`, changing camelCase keys to the documented snake_case keys; see **HCL-safe strings** |
| static override `values` without Legacy `${NAME}` placeholders | nested YAML with the effective Legacy Helm scalar type at the Zarf-mapped source path for each override target path; see **Legacy Helm scalar coercion**, **Literal Go-template delimiters**, and **Override path mapping** |
| static override `values` containing Legacy `${NAME}` placeholders | translate each resolvable scalar placeholder to `{{ .vars.<package>.<normalized_name> }}` with the effective Legacy Helm scalar type at the Zarf-mapped source path, using its collision-safe key when needed; see **Legacy Helm scalar coercion**, **Legacy placeholder translation**, and **Override path mapping** |
| scalar, non-file override `variables` with a configured value or Legacy default | nested YAML at the Zarf-mapped source path using a type-aware `{{ .vars.<package>.<normalized_name> }}` template and collision-safe key when needed; put Legacy defaults in `defaults.uds.hcl` and configured values in `config.uds.hcl` at the mapped scope; see **Legacy Helm scalar coercion** and **YAML scalar rendering** |
| `options.architecture`, `log_level`, `tmp_dir` | same-name fields in `config.uds.hcl` `options` |
| `options.oci_concurrency` | **needs concurrency-semantics review**; do not map automatically to `options.concurrency` |
| legacy `insecure` | manual decision between `plain_http` and `skip_tls_verify`; do not choose automatically |

For each package, stable-deduplicate Legacy `optionalComponents` before emitting
`optional_components`: retain the first occurrence of each exact component name and
remove later identical entries. Record removed duplicates in the migration report so
the Next output remains reviewable while preserving Legacy's effective component
selection. If an entry cannot be read as a component name, mark that package **needs
optional-components review** rather than emitting invalid HCL.

### Development ref overrides

For a Legacy `uds dev deploy --ref <package>=<ref>` override, use the effective ref
instead of the package manifest's `ref` when translating that remote package source.
Legacy resolves a tag override for its effective architecture and uses the resulting
`<ref>@sha256:<digest>` value; preserve that resolved value in the Next OCI source
when supplied. If the override was supplied but its resolved digest is unavailable,
mark the package **needs development-ref source review** rather than silently using
the manifest ref. A Legacy ref override for a local package is invalid; report it as
such and do not invent a Next source. Record each override and the selected effective
ref in the migration report and source-attribution table.

### HCL-safe strings

Encode every migrated literal string as HCL before emitting it, including metadata,
package sources, verification fields, options, and configuration values. In quoted
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
Next configuration supplies a non-nil variables map; an options-only configuration
does not trigger rendering. Determine that map before changing static values. When it
is nil, preserve literal `{{` and `}}` unchanged. Only when it is known to be non-nil,
scan every static Legacy override scalar, list element, and object value for those
delimiters and emit `{{"{{"}}` and `{{"}}"}}`, respectively, so rendering produces the
original delimiter rather than evaluating it. Do not escape intentional Next
expressions generated by this migration, such as `{{ .vars... }}` or `{{ index ... }}`.
If the effective variables map is unknown, it is unclear whether a source delimiter
is literal or intended for Next rendering, or escaping cannot preserve the YAML value
shape, mark it **needs literal-template review** instead of classifying the override
as safely transcribed.

Legacy `oci_concurrency` limits remote OCI layer operations and defaults to `3`.
Next `options.concurrency` is limited to `1` through `25` and controls both remote
OCI package pulls during bundle creation and source deployment, and concurrent package
deployment within a dependency level. It is therefore not an OCI-only equivalent.
Report Legacy `oci_concurrency` as **needs concurrency-semantics review** and write a
Next `concurrency` value only when the user explicitly selects the intended balance
between remote-pull and package-deployment parallelism.

Normalize Legacy override variable names to lowercase snake case (for example,
`REPLICA_COUNT` becomes `replica_count`) for package-scoped values-file templates.
Only top-level scalar values are passed through to Zarf package-variable
substitutions; direct Zarf inputs retain their Legacy uppercase identity as described
in **Direct Zarf package variables**.

Before snake-case allocation, group each package's Legacy override variable names by
their uppercase identity. Names such as `foo` and `FOO` are aliases in Legacy and
must not receive independent Next inputs. When the aliases have one effective scalar
configured value, map every alias to one shared Next key and value. When they have no
configured value, do the same only if their defaults are all absent or semantically
identical. If aliases have differing defaults, a non-scalar or file value, direct
Zarf consumption, or another ambiguous source, mark the group **needs
case-alias-variable review** rather than splitting or silently choosing a value.
Record every Legacy alias and its shared key or review disposition in the
source-attribution table.

After resolving uppercase aliases, detect distinct remaining Legacy identities that
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

Inspect the supplied package's `zarf.yaml` and available package layout for deploy-time
Zarf variable consumers: package-level `variables` declarations, chart `variables`
mappings, and `###ZARF_VAR_NAME###` usages in manifests, templates, actions, or other
deploy inputs. Do not use `###ZARF_PKG_VAR_*###` as this check: it is a package
creation-time template prefix, not the deploy-variable syntax. A Legacy package-scoped
config value used only by a generated values-file template remains under its package
object. A configured scalar whose uppercase Legacy name matches a declared or consumed
deploy-time Zarf variable must also be a collision-free top-level `variables` entry in
`config.uds.hcl`, because Next forwards only top-level scalars to Zarf's
package-variable map.

For each direct deploy variable, use a top-level key whose uppercase form exactly
equals the Zarf variable name. Preserve the Legacy variable spelling for that key when
it provides the required identity: for example, Legacy `fooBar` consumed by
`###ZARF_VAR_FOOBAR###` or a chart variable named `FOOBAR` becomes top-level `fooBar`,
not normalized `foo_bar`. Retain a separately normalized package-scoped entry only
when generated values-file templates also need it. Before lifting, compare every
migrated package that declares or consumes the same Zarf variable. Lift automatically
only when it is the sole consumer or every consumer has the same explicitly configured
scalar Legacy value. If another package has a different value or relies on its Zarf
default or prompt, do not choose one or rename either value: mark the shared scope as
**needs Zarf variable-scope review**. Likewise, if `zarf.yaml` cannot be inspected,
the input is non-scalar, the Legacy uppercase name does not match the declared or
consumed variable, lifting would change the scope, or a direct variable collides with
another top-level value, mark it **needs Zarf variable-scope review** in the migration
report; do not silently leave it nested or choose a renamed fallback.

### Legacy Helm scalar coercion

Legacy sends scalar overrides to Helm as `path=value` and Helm applies its `strvals`
coercion after Legacy YAML parsing and placeholder substitution. Before writing a
static scalar or a scalar override variable/default to Next HCL or YAML, derive that
effective Helm type rather than preserving YAML quotation alone. For example, a Legacy
quoted string `"true"`, `"false"`, `"null"`, or `"42"` becomes the boolean, null, or
integer Helm value and must be emitted as that YAML/HCL type in the Next output.

Apply this conversion before **YAML scalar rendering**. Preserve a string only when
the equivalent Legacy `strvals` input remains a string. If escaping, an unsupported
scalar form, a template result, or an uncertain `strvals` outcome prevents an exact
derivation, mark the override **needs Helm scalar-coercion review** rather than
assuming the source YAML type is equivalent.

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

When a scalar Legacy override variable has neither a configured value nor a Legacy
default, omit its generated values entry so the chart default remains in effect.
Record it as **preserved unset override** in the migration report. Do not emit an
unconditional `{{ .vars... }}` reference: Next renders values templates with missing
keys as errors. If the user needs an optional Next configuration value instead,
mark that behavior **needs optional-value design** rather than choosing a fallback.

For every safely representable Legacy override default, write the mapped variable and
its default value to `defaults.uds.hcl` so the created artifact retains Legacy
fallback behavior. When the Legacy config also supplies a value, write that value at
the same mapped scope in `config.uds.hcl`, where it overrides the artifact default.
Do not omit or relocate a Legacy default merely because the user has not separately
identified it as portable; require an explicit user decision to change that behavior.
Record the default, configured value (when any), and their precedence in the migration
report. Apply sensitive-value handling before writing a sensitive default.

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
instead. Scan every static override value recursively for `${NAME}` and replace it
only when the corresponding name has an actual Legacy effective substitution input
(imported, shared, configured, environment, or CLI value) that has an unambiguous
package-scoped Next configuration representation. A chart variable `default` is not a
substitution input: Legacy applies that default only when processing the variable's
own override, after static-placeholder expansion. Do not use a default alone to
resolve a static placeholder. For an eligible input, replace it with:

```text
{{ .vars.<package>.<normalized_name> }}
```

Keep the replacement in the same scalar, list item, or object property so the
generated YAML preserves the surrounding value shape. Add the variable to the
package-scoped `config.uds.hcl` values and cite both the static override location and
the variable source in the migration report. Apply **Package-scoped template access**
when either generated key is not a Go-template identifier. Use the collision-safe key
allocated for that Legacy variable, when applicable.

Do not translate a placeholder whose variable is absent from the effective Legacy
substitution inputs, exists only as a chart-variable default, is complex or file-backed,
or whose use in a YAML key or mixed-type value makes the resulting YAML ambiguous.
Mark it **needs Legacy placeholder review** and retain the literal source text only in
the report or a clearly labelled comment; do not present a literal `${NAME}` as a
working Next values-file value.

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
use the Legacy name as a generated directory, values-file path, configuration key, or
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
package-scoped configuration applies to every repeated instance, duplicate it for
each generated instance label; preserve distinct Legacy overrides with their
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

If the optional Legacy `uds-config.yaml` sets
`options.skip_signature_validation: true`, it bypasses verification for every Legacy
package, including packages that declare public-key or keyless constraints. Next has
no equivalent bundle-wide setting. Do not report retained verification blocks as an
equivalent migration or silently add `verify = false`. For each affected package,
require the user to make and record an explicit security-labelled choice: retain its
verification posture (a behavior change), set `verify = false` as a local-alpha,
security-reducing adaptation, or leave the canonical migration blocked pending
review. Propagate only a user-selected `verify = false` to that package's
`signature_verification` block and record the Legacy global bypass and each selected
per-package posture in the migration report.

Never select, uncomment, or replace any option in this template on the user's behalf,
including for a local test. Bundle artifact signing with `--unsigned` does not
authorize `verify = false`; they are independent decisions. Ask the user to choose a
package-verification posture before a validation that requires one.

When materializing the template, use `uds bundle create .` and state that the command
must be run from the generated bundle directory. This keeps the command executable
without a synthetic `<bundle-directory>` placeholder. For each of the three commands,
append `--config config.uds.hcl` whenever that file was generated. If it does not set
`options.architecture`, also append `--architecture <effective-legacy-architecture>`;
otherwise omit that flag. Never name a nonexistent config file. In the migration
report, also give the equivalent command from the user's current directory using the
actual output directory path (for example, `./.next`) and the same conditional flags.

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
  # CLI_FEATURES=NextMode=true uds bundle create . --config config.uds.hcl --architecture <effective-legacy-architecture> --signing-key <private-key-or-kms-uri>

  # Option 2: keyless verification. Uncomment the following four lines to select this option.
  # keyless {
  #   certificate_identity_regexp = "https://..."
  #   certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
  # }
  # From this bundle directory, sign the created artifact with an OIDC identity:
  # CLI_FEATURES=NextMode=true uds bundle create . --config config.uds.hcl --architecture <effective-legacy-architecture> --keyless

  # Option 3: local-alpha only; disables package verification. Uncomment only the following line to select this option.
  # verify = false
  # From this bundle directory, create an unsigned artifact:
  # CLI_FEATURES=NextMode=true uds bundle create . --config config.uds.hcl --architecture <effective-legacy-architecture> --unsigned
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
untouched. When the effective Legacy architecture is known, pass it to Zarf so the
prepared archive has the same architecture as the canonical migrated package; for
example. When the Legacy package entry specifies a flavor, also pass that exact
flavor with `--flavor <legacy-flavor>`; otherwise omit the flag:

```sh
mkdir -p .next-validation/packages
CLI_FEATURES=NextMode=true uds tools zarf package create <legacy-local-package-path> --architecture <effective-legacy-architecture> --flavor <legacy-flavor> --output .next-validation/packages --confirm
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
CLI_FEATURES=NextMode=true uds tools zarf package sign .next-validation/packages/<unsigned-archive>.tar.zst --keyless --architecture <effective-legacy-architecture> --output .next-validation/signed-packages --confirm
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
- legacy `valuesFiles` that cannot be folded with their Legacy precedence into a
  package values file and verified against Zarf mappings;
- `shared` configuration, `uds_cache`, `retries`, `UDS_<NAME>` environment variables,
  and `--set` workflows;
- legacy command/flag behavior without a Next equivalent, including `uds logs`,
  `uds list`, deploy `--resume`, `--retries`, `--force-conflicts`, and inspect
  `--sbom`, `--list-images`, and `--list-variables`.

`imports` and `exports` have no direct Next equivalent. Explain the affected values
must instead be supplied through `config.uds.hcl` or values files; do not translate
them to `depends_on` unless the user independently establishes an ordering dependency.

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

Recommend a non-production development deployment only after a successful
`uds bundle create` of the same output directory has enforced every retained package
verification policy. Do not recommend `dev deploy` while a package-verification TODO
is unresolved or after a failed verification; it can continue despite an invalid
package signature. When the user explicitly selected `verify = false` for a
validation copy, label its development deployment as an unverified, local-alpha,
security-reducing workflow rather than validation of the canonical migration. Source
`dev deploy` discovers an adjacent `defaults.uds.hcl` and merges it beneath explicit
`config.uds.hcl` values. Do not recommend deployment of an unsigned artifact unless
the user explicitly authorizes the local-alpha, security-reducing
`--skip-signature-verification` bypass; otherwise require a signed artifact and its
appropriate verification inputs. The report must name the actual generated output
directory rather than relying on the current directory, for example:

```sh
CLI_FEATURES=NextMode=true uds bundle dev deploy <output-dir> --architecture <effective-legacy-architecture> --config <output-dir>/config.uds.hcl
```

Append the `--config` argument whenever the migration generated `config.uds.hcl`;
omit it only when that file was not generated. If the generated config does not set
`options.architecture`, append `--architecture <effective-legacy-architecture>`;
otherwise omit that flag. Next artifacts use `.tar.zst`; source definitions and
artifacts are not backward compatible. Point the user to
`docs/how-to-guides/migrate-legacy-to-next.mdx` in this repository (or the published
Migration guide) for the maintained human walkthrough.

Before declaring the canonical migration ready for review, reconcile its report with
every user-confirmed manual edit while preserving every unresolved blocker the user
has not chosen to resolve. Do not record a validation-copy source substitution or a
test-only `verify = false` selection as a conversion of the canonical migration. A
deliberately selected `verify = false` remains explicitly labelled as local-alpha and
security-reducing in the validation copy's report.
