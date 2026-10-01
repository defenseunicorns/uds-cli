<!-- Generated from the canonical skill and its explicit documentation dependencies. Do not edit. Skill SHA256: 0eeefec4254dc10adf60d6c96b9ee2b7a7397f22c7d9f4c8c8a1219d69ecd8b4 -->

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
| Legacy mappings, override approach, and known gaps | [Legacy-to-Next migration](#migration-doc-docs-how-to-guides-migrate-legacy-to-next-mdx) |
| Next schema, package sources, dependencies, values, and verification | [Bundle HCL reference](#migration-doc-docs-reference-next-bundle-uds-hcl-mdx) |
| Bundle defaults, consumer configuration, and variable precedence | [Configuration HCL reference](#migration-doc-docs-reference-next-config-uds-hcl-mdx) |
| Artifact creation and package preparation | [Create a Next bundle](#migration-doc-docs-how-to-guides-next-create-a-bundle-mdx) |
| Legacy keyless fields and verification constraints | [Legacy keyless package verification](#migration-doc-docs-how-to-guides-verify-keyless-package-signatures-mdx) |
| Package verification versus artifact signing | [Next bundle artifact signing](#migration-doc-docs-how-to-guides-next-sign-bundle-artifacts-mdx) |

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

<a id="migration-doc-docs-how-to-guides-migrate-legacy-to-next-mdx"></a>

## Included documentation: docs/how-to-guides/migrate-legacy-to-next.mdx

<!-- Source SHA256: 8ec78b407e506dcbe0da3c9db692b4cf3029767734bc5d9087e802e8e4055ea6 -->

import { Steps } from '@astrojs/starlight/components';

> [!NOTE]
> UDS CLI Next is an alpha preview. Enable it with `CLI_FEATURES=NextMode=true` for every Next command in this guide.

## What you'll accomplish

Migrate a legacy `uds-bundle.yaml` and `uds-config.yaml` workflow to a Next bundle that you can test, create, and deploy.

## Prerequisites

- [UDS CLI installed](/cli/getting-started/installation/).
- A legacy bundle source directory containing `uds-bundle.yaml`.
- Access to each source package and a non-production cluster with `kubectl` configured for testing.
- Package trust material and bundle signing credentials for your environment.

For an AI-assisted first pass, follow [Migrate a legacy bundle with an AI coding agent](/cli/how-to-guides/migrate-legacy-to-next-with-ai/). It uses the repository's migration skill to generate proposed HCL and a migration report; you must still review package verification and Zarf value mappings before deployment.

> [!WARNING]
> The package references, registry, and key paths in this guide are placeholders. Replace them with sources and trust material that your organization controls. The key files referenced below must exist.

## Steps

<Steps>

1. **Update your command invocations**

   | legacy command | Next command |
   |---|---|
   | `uds create` | `CLI_FEATURES=NextMode=true uds bundle create` |
   | `uds deploy` | `CLI_FEATURES=NextMode=true uds bundle deploy` |
   | `uds dev deploy` | `CLI_FEATURES=NextMode=true uds bundle dev deploy` |
   | `uds inspect` | `CLI_FEATURES=NextMode=true uds bundle inspect` |
   | `uds publish` | `CLI_FEATURES=NextMode=true uds bundle push` |
   | `uds pull` | `CLI_FEATURES=NextMode=true uds bundle pull` |
   | `uds remove` | `CLI_FEATURES=NextMode=true uds bundle remove` or `CLI_FEATURES=NextMode=true uds bundle dev remove` |
   | `uds zarf` | `CLI_FEATURES=NextMode=true uds tools zarf` |
   | `uds run`, `uds monitor`, `uds completion`, `uds version` | unchanged |

   Vendored tools such as `kubectl` and `helm` still use `uds zarf tools <tool>`, not `uds tools zarf tools <tool>`.

   `uds logs`, `uds list`, and the legacy deploy flags `--retries` and `--force-conflicts` have no Next equivalent at the moment. Next supports `--resume`. The legacy inspect flags `--sbom`, `--list-images`, and `--list-variables` are also not available in Next at the moment.

   Next runs non-interactively by default. Use `--prompt` for confirmation.

2. **Convert the bundle definition**

   ```yaml title="legacy/uds-bundle.yaml"
   kind: UDSBundle
   metadata:
     name: podinfo
     description: Podinfo example
     version: 0.1.0

   packages:
     - name: podinfo
       repository: registry.example.com/acme/podinfo
       ref: 1.2.3
       namespace: podinfo
       optionalComponents:
         - ingress
       publicKey: "replace-with-your-package-public-key"
   ```

   Create `next/bundle.uds.hcl`:

   ```hcl title="next/bundle.uds.hcl"
   uds {
     bundle_api_version = "uds.dev/v1alpha1"
   }

   metadata {
     name        = "podinfo"
     description = "Podinfo example"
     version     = "0.1.0"
   }

   package "podinfo" {
     source              = "oci://registry.example.com/acme/podinfo:1.2.3"
     namespace           = "podinfo"
     optional_components = ["ingress"]
     values_files        = ["values/podinfo.yaml"]

     signature_verification {
       public_key = file("keys/package.pub")
     }
   }
   ```

   The main changes are:

   - Remove `kind` and `build`. The `uds` block identifies the Next bundle schema, and Next creates the artifact metadata during artifact creation.
   - Keep the bundle name, description, and version in the `metadata` block. Turn each legacy package entry into a `package "<name>"` block.
   - Combine `repository` and `ref` into `source`, or set `source` to a local Zarf package directory or archive.
   - Rename `optionalComponents` to `optional_components`.
   - Move package signing settings into `signature_verification`. For keyless verification, use `signature_verification.keyless` with the same certificate identity and OIDC issuer constraints.
   - Set architecture with `options.architecture` in `config.uds.hcl` or the `--architecture` flag. It is not a `metadata` setting in Next.
   - Next artifacts use `.tar.zst`. Fields such as `metadata.uncompressed`, package `description`, `timeout`, `flavor`, `imports`, and `exports` have no direct Next equivalent.

3. **Replace component overrides with values files**

   Legacy `overrides` set Helm values inside a component. Next uses [Zarf package values](https://docs.zarf.dev/ref/package-values/) files, referenced with `values_files`. The package's Zarf mappings connect the file to the chart.

   ```yaml title="legacy/uds-bundle.yaml"
   packages:
     - name: podinfo
       overrides:
         podinfo-component:
           unicorn-podinfo:
             variables:
               - name: REPLICA_COUNT
                 path: podinfo.replicaCount
                 default: 1
             values:
               - path: podinfo.service.type
                 value: ClusterIP
   ```

   Create the values file named in `values_files`:

   ```yaml title="next/values/podinfo.yaml"
   podinfo:
     service:
       type: ClusterIP
     replicaCount: {{ .vars.podinfo.replica_count }}
   ```

   The target package must define Zarf mappings for the values you set. For this example, its `zarf.yaml` needs mappings like these:

   ```yaml title="zarf.yaml"
   components:
     - name: podinfo-component
       charts:
         - name: unicorn-podinfo
           values:
             - sourcePath: ".podinfo.replicaCount"
               targetPath: ".replicaCount"
             - sourcePath: ".podinfo.service.type"
               targetPath: ".service.type"
   ```

   Next values files are package-level inputs. Move any legacy `valuesFiles` into the bundle, list them in `values_files`, and give each chart-specific value a matching Zarf mapping. Update and rebuild the Zarf package if a mapping is missing.

   Next sets `namespace` for the whole package, not for individual charts. If charts in one package need different namespaces, update the Zarf package or split the package.

4. **Move configuration into HCL**

   This legacy `uds-config.yaml` sets values for the `podinfo` package and two CLI options:

   ```yaml title="legacy/uds-config.yaml"
   variables:
     podinfo:
       REPLICA_COUNT: 2
   options:
     architecture: amd64
     oci_concurrency: 1
   ```

   Put `defaults.uds.hcl` next to `bundle.uds.hcl`. It provides build-time default values and is included in the bundle artifact.

   ```hcl title="next/defaults.uds.hcl"
   variables = {
     podinfo = {
       replica_count = 1
     }
   }
   ```

   `config.uds.hcl` holds the environment-specific values from `uds-config.yaml`. Pass it with `--config`.

   ```hcl title="next/config.uds.hcl"
   options {
     architecture = "amd64"
     concurrency  = 1
   }

   variables = {
     podinfo = {
       replica_count = 2
     }
   }
   ```

   When moving other settings, use these rules:

   - Keep variables used in `values_files` under the package name. Convert uppercase names to lowercase with underscores, such as `REPLICA_COUNT` to `replica_count`.
   - Zarf package variables must be top-level scalar values and apply to every package. Put shared Helm settings in each package's values file. Use a values file for package-specific settings.
   - Keep `architecture`, `log_level`, and `tmp_dir` in the Next `options` block. Change `oci_concurrency` to `concurrency`. The legacy `--tmpdir` flag becomes `--tmp-dir`.
   - Replace `insecure` with `plain_http` or `skip_tls_verify`, depending on the registry behavior you need. Configure signature verification separately.
   - Next has no equivalents for `uds_cache`, `retries`, or `UDS_<NAME>` environment variables.

   Next applies built-in defaults, then `config.uds.hcl` values, then explicit command-line flags. `config.uds.hcl` overrides matching values in the adjacent `defaults.uds.hcl`, then command-line flags override `config.uds.hcl`.

5. **Test, create, and deploy the Next artifact**

   Test the definition in a non-production cluster first.

   ```bash
   CLI_FEATURES=NextMode=true uds bundle dev deploy ./next --config ./next/config.uds.hcl
   ```

   Create and sign the artifact after the development deployment succeeds:

   ```bash
   cd next
   CLI_FEATURES=NextMode=true uds bundle create . --architecture amd64 --signing-key ./keys/bundle-signing.key
   ```

   Push and deploy the signed artifact:

   ```bash
   CLI_FEATURES=NextMode=true uds bundle push \
     ./uds-bundle-podinfo-amd64-0.1.0.tar.zst \
     oci://registry.example.com/acme/podinfo:0.1.0

   CLI_FEATURES=NextMode=true uds bundle deploy \
     oci://registry.example.com/acme/podinfo:0.1.0 \
     --config ./config.uds.hcl \
     --public-key ./keys/bundle-signing.pub
   ```

</Steps>

## Verification

Confirm the migrated workload is healthy in the target cluster before removing the legacy deployment:

```bash
kubectl get pods -n podinfo
```

Continue when the workload is Ready.

## Known differences and gaps

- Registry transport and signature verification are separate in Next. Use `--plain-http` or `--skip-tls-verify` for registry transport, configure package verification in each package's `signature_verification` block, and use `--skip-signature-verification` only for local alpha testing because it leaves bundle integrity unverified.
- Next cannot use legacy definitions or artifacts. Next has no automatic conversion. Convert the source to HCL, then create a Next artifact.
- Legacy package-to-package variable passing through `imports` and `exports` is not supported in Next. Provide those values through `config.uds.hcl` or package values files.
- Development deployment uses the source definition directly. It does not create an artifact or provide bundle-signature verification. Use the create-then-deploy artifact workflow when you need signed bundle verification.

## Related documentation

- [Migrate a legacy bundle with an AI coding agent](/cli/how-to-guides/migrate-legacy-to-next-with-ai/) - use the migration skill and validate its proposed output
- [Next mode reference](/cli/reference/next-mode/) - exact Next commands, flags, and configuration behavior
- [Bundle overrides](/cli/how-to-guides/use-bundle-overrides/) - Legacy override behavior used in the migration example

<a id="migration-doc-docs-reference-next-bundle-uds-hcl-mdx"></a>

## Included documentation: docs/reference/next/bundle-uds-hcl.mdx

<!-- Source SHA256: be88263cd0c728718397d32c82649a9f8c9c8c9ade5eb0bc56275ebaf88b05e9 -->

`bundle.uds.hcl` is the source definition for a UDS CLI Next bundle. It declares the bundle identity and the Zarf packages that become part of the bundle artifact or are deployed directly in development mode.

The file must be named exactly `bundle.uds.hcl`. It is distinct from `defaults.uds.hcl`, which supplies bundle-level default variables, and `config.uds.hcl`, which supplies consumer-owned deploy-time settings.

## Top-level structure

The top-level blocks identify the bundle and declare its packages.

| Block or attribute | Type | Required | Description |
|---|---|---:|---|
| `uds` | block | Yes | Declares the bundle API version. |
| `metadata` | block | Yes | Declares the bundle name and optional descriptive metadata. |
| `locals` | block | No | Defines reusable HCL expressions. |
| `package` | block | At least one | Declares a Zarf package included in the bundle. Every package block is included. |

Minimal definition:

```hcl title="bundle.uds.hcl"
uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name = "my-bundle"
}

package "app" {
  source = "oci://registry.example.com/my-org/app:1.0.0"

  signature_verification {
    verify = false
  }
}
```

`verify = false` is an explicit package signature bypass for local development. For a trusted create workflow, configure `public_key` or `keyless`.

## `uds`

The `uds` block has one supported attribute:

| Attribute | Type | Required | Valid value |
|---|---|---:|---|
| `bundle_api_version` | string | Yes | Exactly `"uds.dev/v1alpha1"` |

The field is required even when the bundle is deployed directly from its definition. Other API versions are rejected as unsupported.

## `metadata`

The `metadata` block identifies the bundle and provides its descriptive information.

| Attribute | Type | Required | Description |
|---|---|---:|---|
| `name` | string | Yes | Bundle name. It identifies the bundle in command output and artifact metadata. |
| `description` | string | No | Human-readable description. Defaults to an empty string. |
| `version` | string | No | Bundle version. Defaults to an empty string and is used when naming versioned artifacts. |

Bundle validation requires `name` to be non-empty. The parser requires the attribute to be present, and `description` and `version` must be strings when present. The current schema does not apply additional format validation to those two optional fields.

## `locals`

Use `locals` for values shared by metadata and package expressions. Local values are evaluated in dependency order, so one local can reference another with `local.<name>`:

```hcl
locals {
  registry   = "ghcr.io/my-org"
  version    = "1.0.0"
  app_source = "oci://${local.registry}/app:${local.version}"
}

metadata {
  name    = "my-app"
  version = local.version
}

package "app" {
  source = local.app_source
  signature_verification { verify = false }
}
```

The built-in `sys.arch` value contains the effective target architecture. It can be used to select a local package archive:

```hcl
package "app" {
  source = "./packages/app-${sys.arch}.tar.zst"
  signature_verification { verify = false }
}
```

Local names must be unique. References to an undefined local and cyclic local dependencies fail during parsing. The `file(path)` function is also available in a file-backed bundle definition. Relative paths are resolved from the directory containing `bundle.uds.hcl`; absolute paths are used as-is. The function returns regular UTF-8 file contents.

## `package "<id>"`

The package label is the package identifier used in dependency references and bundle output.

| Attribute or block | Type | Required | Default | Description |
|---|---|---:|---|---|
| `<id>` | label | Yes | None | Must be unique. It cannot contain `/` or `\\`, and cannot be `.` or `..`. |
| `source` | string | Yes | None | OCI reference or local Zarf package source. |
| `namespace` | string | No | Empty | Namespace override for the Zarf package. |
| `depends_on` | list of package references | No | Empty | Packages that must deploy before this package. |
| `values_files` | list of strings | No | Empty | Package values files, resolved relative to the bundle definition directory. |
| `optional_components` | list of strings | No | Empty | Optional Zarf components to include or exclude. |
| `signature_verification` | block | Required for `bundle create` | None | Package signature policy applied when the package enters the bundle. |

### `source`

Sources can be OCI package references or local Zarf package paths:

```hcl
package "remote" {
  source = "oci://registry.example.com/my-org/remote:1.0.0"
  signature_verification { verify = false }
}

package "local" {
  source = "./packages/local-${sys.arch}.tar.zst"
  signature_verification { verify = false }
}
```

Relative local paths are resolved from the directory containing `bundle.uds.hcl`. A local source must be a Zarf package directory or a `.tar.zst` package archive.

### `depends_on`

Dependencies use static package references, not quoted strings:

```hcl
depends_on = [package.database, package.platform]
```

Each element must be exactly a `package.<id>` traversal. A dependency must refer to a package declared in the same bundle and cannot refer to its own package. Independent packages can be deployed in parallel, subject to the configured concurrency limit.

### `values_files`

List package values files in the order they should be applied:

```hcl
values_files = [
  "values/base.yaml",
  "values/staging.yaml",
]
```

Relative paths are resolved from the directory containing `bundle.uds.hcl`. Source-directory operations must be able to read the files. During bundle creation, the files are stored in the artifact under `values/<package-id>/`, and artifact deployments use those embedded copies. These are [Zarf package values](https://docs.zarf.dev/ref/package-values/) files. During deployment, Go templates can read deploy-time variables through `.vars`. Deploy-time variables and their precedence are configured in `config.uds.hcl`.

### `optional_components`

When the list is empty, the package's required and default Zarf components are selected. Listing a component selects it alongside required components. Prefix a component with `-` to explicitly exclude it:

```hcl
optional_components = [
  "metrics",
  "-debug-shell",
]
```

Component names must not be empty or repeated. Component selection is not a deploy-time bundle variable.

## `signature_verification`

Each package passed to `uds bundle create` must declare its signature posture. Verification is enabled by default when `verify` is omitted. Configure exactly one verification method when enabled, or use `verify = false` without a method for an explicit bypass.

| Attribute or block | Type | Required | Description |
|---|---|---:|---|
| `verify` | boolean | No | Defaults to `true`. Set to `false` to skip package signature verification. It cannot be combined with `public_key` or `keyless`. |
| `public_key` | string | One method when verifying | Public key contents. Use `file("keys/package-signer.pub")` to load a key file. |
| `keyless` | block | One method when verifying | Keyless certificate and issuer constraints. |

Keyless verification supports the following fields:

| Attribute | Type | Required | Description |
|---|---|---:|---|
| `certificate_identity` | string | One identity | Exact certificate identity. |
| `certificate_identity_regexp` | string | One identity | Regular expression for certificate identities. |
| `certificate_oidc_issuer` | string | One issuer | Exact OIDC issuer. |
| `certificate_oidc_issuer_regexp` | string | One issuer | Regular expression for OIDC issuers. |
| `trusted_root` | string | No | Sigstore trusted-root JSON contents. The embedded Sigstore root is used when omitted. |
| `insecure_ignore_tlog` | boolean | No | Defaults to `false`. Disables transparency-log verification. |
| `insecure_ignore_sct` | boolean | No | Defaults to `false`. Disables certificate SCT verification. |
| `use_signed_timestamps` | boolean | No | Defaults to `false`. Enables signed-timestamp verification when applicable. |

Use exactly one identity form and exactly one issuer form. Regular expressions must compile. The insecure options reduce verification protections and should be reserved for an intentional, documented trust decision.

Example keyless package policy:

```hcl
package "app" {
  source = "oci://registry.example.com/my-org/app:1.0.0"

  signature_verification {
    keyless {
      certificate_identity_regexp = "https://github\\.com/my-org/.*/.github/workflows/release\\.yml@refs/heads/main"
      certificate_oidc_issuer      = "https://token.actions.githubusercontent.com"
    }
  }
}
```

Direct development deployment attempts package verification and warns instead of stopping when the policy would fail `bundle create`. Artifact deployment does not reverify contained package signatures. Bundle artifact signing and verification are separate controls.

## Related documentation

- [Next mode reference](/cli/reference/next-mode/) - Overview of Next mode behavior and commands.
- [Sign and verify a bundle in Next mode](#migration-doc-docs-how-to-guides-next-sign-bundle-artifacts-mdx) - Task-oriented workflow for signing, verifying, publishing, and deploying bundle artifacts.

<a id="migration-doc-docs-reference-next-config-uds-hcl-mdx"></a>

## Included documentation: docs/reference/next/config-uds-hcl.mdx

<!-- Source SHA256: 5c66892e36e9ea97a33158f47f921a027e66edd25222a612b668a6f7b7b5d0ac -->

`config.uds.hcl` supplies consumer-owned settings when UDS CLI Next operates on a bundle. It is read with `--config` and is not part of the bundle artifact. Use it for settings that can change between environments, such as registry options, signature trust material, and deploy-time variables.

This file is distinct from `bundle.uds.hcl`, which defines the bundle, and `defaults.uds.hcl`, which stores bundle-provided default variables.

## Supported top-level attributes

The file supports these top-level attributes and blocks.

| Attribute or block | Type | Required | Default | Purpose |
|---|---|---:|---|---|
| `options` | block | No | Operation defaults | Configure CLI operation options. |
| `signature_verification` | block | No | No policy | Configure trust for bundle artifact signatures. |
| `variables` | object | No | Omitted | Supply deploy-time values to the bundle. |

Minimal example:

```hcl title="config.uds.hcl"
variables = {
  environment = "development"
}

signature_verification {
  public_key = file("keys/cosign.pub")
}
```

Use the file with `--config`:

```bash
CLI_FEATURES=NextMode=true uds bundle deploy \
  ./uds-bundle-my-app-amd64-1.0.0.tar.zst \
  --config ./config.uds.hcl
```

## `options`

The `options` block configures operation-wide behavior. Config values override built-in defaults and yield to explicit CLI flags.

| Attribute | Type | Effective default | Validation and behavior |
|---|---|---|---|
| `log_level` | string | `"info"` | Accepts `debug`, `info`, `warn`, or `error`. `warning` is accepted as an alias for `warn`. |
| `architecture` | string | The current runtime architecture | Selects the target architecture for architecture-aware bundle and package operations. |
| `plain_http` | boolean | `false` | Allows registry communication over plain HTTP. Use only with a registry that intentionally does not provide TLS. |
| `skip_tls_verify` | boolean | `false` | Disables TLS certificate verification for registry communication. This reduces transport security. |
| `tmp_dir` | string | The operating system temporary directory | Must name an existing directory when set. |
| `concurrency` | integer | `10` | Values from `1` to `25` are accepted. `0` is treated as unset and uses the default of `10`. |

Example:

```hcl title="config.uds.hcl"
options {
  log_level       = "debug"
  architecture    = "amd64"
  concurrency     = 5
}
```

The `tmp_dir` directory must already exist before running the command.

## `signature_verification`

This block defines the consumer's trust policy for a bundle artifact. It is separate from the package signature policy in each `package` block of `bundle.uds.hcl`, and it is separate from the signing options used by `bundle create` and `bundle sign`.

Configure exactly one verification method:

```hcl title="config.uds.hcl"
signature_verification {
  public_key = file("keys/cosign.pub")
}
```

| Attribute or block | Type | Required | Description |
|---|---|---:|---|
| `public_key` | string | One method | Public key contents used to verify a key-signed bundle. Use `file()` when the key is stored in a file. |
| `keyless` | block | One method | Constraints for a keyless certificate and its OIDC issuer. |
| `keyless.certificate_identity` | string | One identity | Exact certificate identity to trust. |
| `keyless.certificate_identity_regexp` | string | One identity | Regular expression for certificate identities to trust. |
| `keyless.certificate_oidc_issuer` | string | One issuer | Exact OIDC issuer to trust. |
| `keyless.certificate_oidc_issuer_regexp` | string | One issuer | Regular expression for OIDC issuers to trust. |
| `keyless.trusted_root` | string | No | Sigstore trusted-root JSON contents. If omitted, the embedded Sigstore root is used. |

Exact and regular-expression forms are mutually exclusive for both identity and issuer. A keyless policy must contain exactly one identity form and exactly one issuer form:

```hcl title="config.uds.hcl"
signature_verification {
  keyless {
    certificate_identity_regexp = "https://github\\.com/my-org/my-repo/.github/workflows/release\\.yml@refs/heads/main"
    certificate_oidc_issuer      = "https://token.actions.githubusercontent.com"
    trusted_root                 = file("keys/trusted-root.json")
  }
}
```

For `inspect`, `verify`, `pull`, `deploy`, `reconfigure`, and artifact-based `remove`, CLI verification flags override matching configured fields. Exact and regular-expression identity or issuer forms must not be mixed between the CLI and this block; use the same form in both places or omit the conflicting configured field. `--skip-signature-verification` is an explicit insecure bypass where the command supports it. `bundle create` and `bundle sign` use signing flags instead of this consumer trust policy.

## `variables`

`variables` must be an HCL object. Values can be strings, numbers, booleans, nested objects, or lists and tuples of supported values.

```hcl title="config.uds.hcl"
variables = {
  environment     = "staging"
  replica_count   = 3
  features = {
    audit_logs = true
  }
  allowed_regions = ["us-east-1", "us-west-2"]
}
```

Variables are used by `bundle dev deploy` and `bundle deploy`. Top-level scalar values are also exposed to Zarf as uppercase variable names. Nested objects and collection values are intended for [Zarf package values](https://docs.zarf.dev/ref/package-values/) file templates.

A package values file can read variables with Go template expressions:

```yaml title="values/app.yaml"
replicas: {{ .vars.replica_count }}
environment: "{{ .vars.environment }}"
auditLogs: {{ .vars.features.audit_logs }}
```

When the resolved configuration contains variables, the values file is rendered at deployment time. A referenced variable that is missing from the merged variable set causes deployment to fail. Values files pass through without UDS variable templating only when every configuration layer omits the `variables` attribute. An explicit empty object (`variables = {}`) still renders values files.

Config variables do not change the bundle definition, package sources, package signature policies, or the set of packages in the artifact.

## Precedence and timing

The two configuration categories resolve independently:

| Category | Lowest precedence | Higher precedence | Highest precedence |
|---|---|---|---|
| Operation options | Built-in defaults | `config.uds.hcl` `options` | Explicit CLI flags |
| Bundle variables | Adjacent or embedded `defaults.uds.hcl` | `config.uds.hcl` `variables` | Deploy `--set` flags |

For variables, nested objects are deep-merged. A scalar or collection in a higher-precedence layer replaces the corresponding lower-precedence value as a whole. For an artifact deployment, defaults embedded in the artifact are the base layer. For `bundle dev deploy`, the adjacent `defaults.uds.hcl` file is the base layer.

Both `bundle deploy` and `bundle dev deploy` accept repeatable `--set key=value` flags. Plain values are strings, while booleans, numbers, HCL lists, and HCL objects retain their types. Quote ambiguous values to force strings:

```bash
CLI_FEATURES=NextMode=true uds bundle dev deploy ./my-bundle \
  --set domain=example.com \
  --set replicas=3 \
  --set enabled=true \
  --set 'ports=[8080, 8443]' \
  --set release=\"3\" \
  --set 'database={ host = "db.example", port = 5432 }'
```

`defaults.uds.hcl` has a narrower schema. It may contain only a top-level `variables` attribute and cannot contain `options` or `signature_verification` blocks. Options and bundle signature trust remain consumer settings in `config.uds.hcl`.

The config file is evaluated when the command reads it. It is not embedded into a created artifact, so provide it again when a later deployment needs its options or variables.

## `file()`

`file(path)` reads a regular UTF-8 file. Relative paths are resolved from the directory containing `config.uds.hcl`; absolute paths are used as-is. It returns the contents as a string and is useful for public keys and trusted-root JSON:

```hcl
signature_verification {
  public_key = file("keys/cosign.pub")
}
```

The path must identify an existing regular UTF-8 file. Missing files, directories, and invalid UTF-8 cause config evaluation to fail.

## Command consumers

These settings apply to the following bundle commands.

| Config content | Commands that consume it | Effect |
|---|---|---|
| `options` | All `uds bundle` commands that resolve common options | Sets logging, architecture, registry transport, temporary directory, and concurrency behavior. |
| `variables` | `bundle dev deploy`, `bundle deploy` | Renders package values files and supplies supported top-level scalar Zarf variables. |
| `signature_verification` | `bundle inspect`, `bundle verify`, `bundle pull`, `bundle deploy`, `bundle reconfigure`, `bundle remove` (artifact sources only) | Supplies the default consumer policy for bundle artifact signature verification. |

`bundle create` uses package verification settings from each package block and signing flags from the command line. `bundle sign` uses signing flags and operation options only; it does not use package verification settings.

## Related documentation

- [Next mode reference](/cli/reference/next-mode/) - Overview of Next mode behavior and commands.
- [Sign and verify a bundle in Next mode](#migration-doc-docs-how-to-guides-next-sign-bundle-artifacts-mdx) - Task-oriented workflow for signing, verifying, publishing, and deploying bundle artifacts.

<a id="migration-doc-docs-how-to-guides-next-create-a-bundle-mdx"></a>

## Included documentation: docs/how-to-guides/next/create-a-bundle.mdx

<!-- Source SHA256: 3a73f31394777f4b772712ad5d13b753af2f91dff45a60e9053e5e29404223f9 -->

import { Steps } from '@astrojs/starlight/components';

Create a self-contained OCI artifact from a `bundle.uds.hcl` definition.

## What you'll accomplish

- Define package sources and dependencies.
- Add package values and bundle defaults.
- Create a signed artifact or an unsigned local test artifact.

## Prerequisites

- [UDS CLI installed](/cli/getting-started/installation/)
- A directory containing the bundle definition
- OCI access to each package source
- Public key files referenced by package verification policies
- A Cosign signing key or an OIDC identity when creating a signed artifact

## Steps

<Steps>

1. **Define the bundle**

   Create `bundle.uds.hcl`:

   The `my-org`, package names, package references, key paths, and signer identity below are placeholders. Replace them with package sources and trust material available in your environment.

   ```hcl title="bundle.uds.hcl"
   uds {
     bundle_api_version = "uds.dev/v1alpha1"
   }

   metadata {
     name        = "my-app"
     description = "My application bundle"
     version     = "1.0.0"
   }

   package "database" {
     source = "oci://ghcr.io/my-org/packages/postgres:15.0.0"

     signature_verification {
       public_key = file("keys/my-org.pub")
     }
   }

   package "api" {
     source       = "oci://ghcr.io/my-org/packages/my-api:2.0.0"
     depends_on   = [package.database]
     values_files = ["values/api.yaml"]

     signature_verification {
       keyless {
         certificate_identity    = "https://github.com/my-org/api/.github/workflows/release.yml@refs/heads/main"
         certificate_oidc_issuer = "https://token.actions.githubusercontent.com"
       }
     }
   }
   ```

   Create the values file referenced by the `api` package. Values files support Go templates. The `{{ .vars.* }}` expressions read variables available at deploy time, including values from `defaults.uds.hcl` or `config.uds.hcl`; missing variables cause deployment to fail. In this example, the package maps the `replicas` value to the chart's `replicaCount` value:

   ```yaml title="values/api.yaml"
   replicas: {{ .vars.replica_count }}
   ```

   The package author defines that mapping in the package's `zarf.yaml`:

   ```yaml title="zarf.yaml"
   kind: ZarfPackageConfig
   metadata:
     name: api
     version: 1.0.0

   values:
     files:
       - values.yaml

   components:
     - name: api
       required: true
       charts:
         - name: api
           version: 1.0.0
           namespace: api
           url: oci://ghcr.io/my-org/charts/api
           values:
             - sourcePath: ".replicas"
               targetPath: ".replicaCount"
   ```

   `sourcePath` identifies the key in the Zarf values file, and `targetPath` identifies the corresponding Helm chart value. At deploy time, the `replica_count` bundle variable becomes the Zarf `replicas` value, which Zarf maps to the chart's `replicaCount` value.

    The package ID is the label after `package`. Use it with `depends_on` to control deployment order, for example `depends_on = [package.database]`. Independent packages deploy in parallel.

    Each package needs one verification method: `public_key`, `keyless`, or an explicit `signature_verification { verify = false }` bypass. Use the bypass only for local alpha workflows. It produces a warning.

2. **Use defaults and locals**

   Put environment-independent defaults in `defaults.uds.hcl` next to the bundle definition:

   ```hcl title="defaults.uds.hcl"
   variables = {
     cluster_name  = "development"
     replica_count = 1
   }
   ```

    Use `locals` to avoid repeating registry or version values:

    ```hcl title="bundle.uds.hcl"
    locals {
      package_registry = "ghcr.io/my-org/packages"
      api_version      = "2.0.0"
    }

    package "api" {
      source = "oci://${local.package_registry}/my-api:${local.api_version}"
      signature_verification { verify = false }
    }
    ```

    Use `file(path)` for UTF-8 text files. Paths resolve relative to the HCL file.

3. **Create the artifact**

   Create a signed artifact using a local Cosign key:

   ```bash
   CLI_FEATURES=NextMode=true uds bundle create . --signing-key ./cosign.key
   ```

   For a local unsigned test artifact:

   ```bash
   CLI_FEATURES=NextMode=true uds bundle create . --unsigned
   ```

    Use `--keyless` to sign through an OIDC identity. The command writes a local `.tar.zst` containing the definition, defaults, values, and package content.

</Steps>

## Verification

Confirm that the artifact exists and contains the bundle metadata:

```bash
CLI_FEATURES=NextMode=true uds bundle inspect ./uds-bundle-my-app-<ARCH>-1.0.0.tar.zst
```

Replace `<ARCH>` with the target architecture, such as `amd64` or `arm64`.

## Related documentation

- [Deploy a bundle in Next mode](/cli/how-to-guides/next/deploy-a-bundle/) - Deploy a definition or created artifact.
- [Next mode reference](/cli/reference/next-mode/) - Review the complete HCL and command reference.

<a id="migration-doc-docs-how-to-guides-verify-keyless-package-signatures-mdx"></a>

## Included documentation: docs/how-to-guides/verify-keyless-package-signatures.mdx

<!-- Source SHA256: 5d0bdde1d8d07b50f047c5a98bcec23c282ed11573bc348e7428f9f1e43ccf8c -->

import { Steps } from '@astrojs/starlight/components';

## What you'll accomplish

UDS CLI validates package signatures when it creates, inspects, or deploys packages from a bundle. By the end of this guide you'll know how to:

- Verify a package with Sigstore keyless certificate identity and issuer constraints
- Configure exact-match or regex-based keyless constraints
- Add advanced keyless options when your environment requires them
- Disable package signature validation only when you explicitly accept that risk

## Prerequisites

- [UDS CLI installed](/getting-started/installation/)
- A `uds-bundle.yaml` referencing one or more keyless-signed Zarf packages
- The expected signing certificate identity and OIDC issuer from the package publisher

## Before you begin

Keyless package signature verification is configured per package in `uds-bundle.yaml` with `keylessVerification`. Configuring it requires that package to be signed; UDS CLI rejects an unsigned package rather than silently skipping verification.

> [!NOTE]
> Each package can use one verification method. For keyless-signed packages, configure `keylessVerification` and leave `publicKey` unset.

Keyless verification requires one certificate identity constraint and one OIDC issuer constraint. Use exact-match fields when the publisher's signing identity is stable. Use regex fields when the trusted identity changes predictably, such as a release workflow identity that includes a version tag.

```yaml
packages:
  - name: example
    repository: ghcr.io/example/package
    ref: 1.0.0
    keylessVerification:
      certificateIdentity: https://github.com/example/repo/.github/workflows/release.yml@refs/tags/v1.0.0
      certificateOIDCIssuer: https://token.actions.githubusercontent.com
```

> [!CAUTION]
> Keep keyless constraints specific to the publisher identities you trust. Broad regex patterns can match more signing identities than intended.

## Steps

<Steps>

1. **Get the expected signing identity**

   Ask the package publisher for the certificate identity and OIDC issuer used to sign the package. For GitHub Actions keyless signing, the issuer is commonly `https://token.actions.githubusercontent.com`.

2. **Configure keyless verification**

   Add `keylessVerification` to the package entry with certificate identity and OIDC issuer constraints:

   ```yaml title="uds-bundle.yaml"
   kind: UDSBundle
   metadata:
     name: keyless-package-example
     version: 0.0.1

   packages:
     - name: init
       repository: ghcr.io/zarf-dev/packages/init
       ref: v0.86.0
       keylessVerification:
         certificateIdentityRegexp: https://github\.com/zarf-dev/zarf/\.github/workflows/release\.yml@refs/tags/v\d+\.\d+\.\d+
         certificateOIDCIssuer: https://token.actions.githubusercontent.com
   ```

   Keyless verification requires one identity field and one issuer field:

   | Required constraint | Exact match | Regex match |
   |---|---|---|
   | Certificate identity | `certificateIdentity` | `certificateIdentityRegexp` |
   | OIDC issuer | `certificateOIDCIssuer` | `certificateOIDCIssuerRegexp` |

   Use exact matches when the signing identity is stable. Use regex matches when the trusted identity changes predictably, such as a release workflow identity that includes a version tag.

3. **Add advanced keyless options only when needed**

   Most keyless verification should use the default trusted root and transparency log verification. Add these fields only when the package publisher or your environment requires them:

   ```yaml title="uds-bundle.yaml"
   packages:
     - name: private-package
       repository: registry.example.com/packages/private-package
       ref: 1.0.0
       keylessVerification:
         certificateIdentity: https://github.com/example/repo/.github/workflows/release.yml@refs/tags/v1.0.0
         certificateOIDCIssuer: https://token.actions.githubusercontent.com
         trustedRoot: |
           <sigstore-trusted-root-json>
         useSignedTimestamps: true
         insecureIgnoreTlog: true
   ```

   > [!CAUTION]
   > Set `insecureIgnoreTlog: true` only when your environment cannot use Rekor transparency log verification, such as private or air-gapped Sigstore infrastructure.

4. **Run a UDS CLI command that verifies package signatures**

   Package signatures are verified automatically during bundle create, inspect, and deploy operations unless signature validation is skipped. Use the commands in the Verification section to confirm the configuration works.

</Steps>

## Verification

Inspect the bundle configuration before creating the bundle:

```bash
uds inspect uds-bundle.yaml
```

Create the bundle:

```bash
uds create . --confirm
```

Deploy the bundle without `--skip-signature-validation`:

```bash
uds deploy uds-bundle.tar.zst --confirm
```

If verification fails, UDS CLI stops before continuing with the package operation.

To bypass package signature verification, use `--skip-signature-validation`:

```bash
uds inspect uds-bundle.yaml --skip-signature-validation
```

> [!CAUTION]
> Skipping signature validation bypasses this verification step. Use it only when you have a specific reason and trust the package source.

## Troubleshooting

### Problem: Keyless verification is missing identity or issuer

**Symptom:** UDS CLI returns an error saying keyless verification requires certificate identity or OIDC issuer configuration.

**Solution:** Set one identity field (`certificateIdentity` or `certificateIdentityRegexp`) and one issuer field (`certificateOIDCIssuer` or `certificateOIDCIssuerRegexp`).

### Problem: Package is signed but keyless verification is not configured

**Symptom:** UDS CLI returns an error saying the package is signed but no verification material was provided.

**Solution:** Add `keylessVerification` for the signed package, or use `--skip-signature-validation` only if you have verified the package through another trusted process.

## Related Documentation

- [uds inspect command reference](/reference/commands/uds_inspect/)
- [uds create command reference](/reference/commands/uds_create/)
- [uds deploy command reference](/reference/commands/uds_deploy/)
- [Zarf package verify](https://docs.zarf.dev/commands/zarf_package_verify/)
- [Zarf Package Signing](https://docs.zarf.dev/ref/package-signing/)
- [Schema Validation & IDE Setup](/reference/schema-validation/)

<a id="migration-doc-docs-how-to-guides-next-sign-bundle-artifacts-mdx"></a>

## Included documentation: docs/how-to-guides/next/sign-bundle-artifacts.mdx

<!-- Source SHA256: 4eff5dfb7b889c72c7f203ea4b2efe5a1df5e23ef80327f5bfadf9a32bc3ef57 -->

import { Steps } from '@astrojs/starlight/components';

Use bundle artifact signatures to verify an artifact's origin and integrity against a trusted key or identity.

## What you'll accomplish

Use this guide to create, verify, and distribute signed UDS CLI Next bundle artifacts. It covers key-based and OpenID Connect (OIDC) signing.

- Generate Cosign-compatible signing material with Zarf.
- Create or sign a bundle artifact with a key or an OpenID Connect (OIDC) identity.
- Verify, publish, pull, and deploy a signed artifact.

## Prerequisites

- [UDS CLI installed](/cli/getting-started/installation/)
- [Zarf installed](https://docs.zarf.dev/ref/install/)
- A valid Next mode bundle directory containing `bundle.uds.hcl`
- Access to the package sources and, for publishing, an OCI registry
- A configured OIDC provider for keyless signing, or permission to create and protect a signing key
- A Kubernetes cluster and deployment configuration when deploying

Every UDS CLI command in this guide enables Next mode with `CLI_FEATURES=NextMode=true`.

## Steps

<Steps>

1. **Prepare signing and package verification material**

   Generate a Cosign key pair with Zarf. Run this command from a protected directory, not from a source repository:

   ```bash
   umask 077
   uds zarf tools gen-key
   ```

   The command creates `cosign.key`, the private signing key, and `cosign.pub`, the public verification key. Store `cosign.key` in a secret manager or another access-controlled location. Do not commit or share it. Distribute `cosign.pub` to artifact verifiers.

   Bundle artifact signing and package signature verification are separate controls. Before `uds bundle create`, each package in `bundle.uds.hcl` must either trust its package signer or explicitly opt out for local testing. For a key-based package signature, use the public key that signed the package:

   ```hcl title="bundle.uds.hcl"
   package "app" {
     source = "oci://registry.example.com/my-org/app:1.0.0"

     signature_verification {
       public_key = file("keys/package-signer.pub")
     }
   }
   ```

   For a keyless package signature, configure one certificate identity constraint and one OIDC issuer constraint instead.

2. **Create a signed artifact with a private key**

   Use this step or step 3 to sign during creation. If the key is password-protected, set `PRIVATE_KEY_PASSWORD` from a secret manager and pass it through `COSIGN_PASSWORD`, not a command-line argument:

   ```bash
   COSIGN_PASSWORD="$PRIVATE_KEY_PASSWORD" \
   CLI_FEATURES=NextMode=true uds bundle create ./my-bundle \
     --signing-key ./cosign.key
   ```

   The command writes a signed `.tar.zst` artifact beside `bundle.uds.hcl`, for example `./my-bundle/uds-bundle-my-app-amd64-1.0.0.tar.zst`. The signature covers the bundle definition, defaults, values, package manifests, and package content.

3. **Create a signed artifact with keyless signing**

   Use keyless signing when the build environment can obtain an OIDC identity token:

   ```bash
   CLI_FEATURES=NextMode=true uds bundle create ./my-bundle --keyless
   ```

   Record the certificate identity and issuer so deployers can configure matching verification constraints.

4. **Sign an existing artifact**

   Use this step for an existing unsigned artifact. If you completed step 2 or 3, skip to step 5. `bundle sign` adds evidence to an existing local artifact or OCI bundle reference. For a local artifact, it updates the archive in place. Add `--overwrite` when replacing an existing signature.

   For a multi-architecture OCI tag, `bundle sign` signs only the child selected by `--architecture`, which defaults to the host architecture. Run the command once per architecture, or sign a single-architecture reference.

   ```bash
   COSIGN_PASSWORD="$PRIVATE_KEY_PASSWORD" \
   CLI_FEATURES=NextMode=true uds bundle sign ./my-bundle/uds-bundle-my-app-amd64-1.0.0.tar.zst \
     --signing-key ./cosign.key
   ```

   To replace an existing key-based signature:

   ```bash
   COSIGN_PASSWORD="$PRIVATE_KEY_PASSWORD" \
   CLI_FEATURES=NextMode=true uds bundle sign ./my-bundle/uds-bundle-my-app-amd64-1.0.0.tar.zst \
     --signing-key ./cosign.key \
     --overwrite
   ```

   Use `--keyless` for keyless signing:

   ```bash
   CLI_FEATURES=NextMode=true uds bundle sign ./my-bundle/uds-bundle-my-app-amd64-1.0.0.tar.zst --keyless
   ```

5. **Verify the artifact before distribution**

   Verify a key-signed artifact with the matching public key:

   ```bash
   CLI_FEATURES=NextMode=true uds bundle verify ./my-bundle/uds-bundle-my-app-amd64-1.0.0.tar.zst \
     --public-key ./cosign.pub
   ```

   Verify a keyless artifact by constraining both the certificate identity and OIDC issuer. Use the exact identity issued by your provider:

   ```bash
   CLI_FEATURES=NextMode=true uds bundle verify ./my-bundle/uds-bundle-my-app-amd64-1.0.0.tar.zst \
     --certificate-identity 'https://github.com/my-org/my-repo/.github/workflows/release.yml@refs/heads/main' \
     --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
   ```

   Use the regexp variants when the trusted identity or issuer has a controlled pattern. Keyless verification checks transparency-log inclusion and certificate SCT evidence by default. Use `--trusted-root` to select a specific Sigstore trusted-root JSON file.

   Verification checks the bundle signature and complete OCI graph. Changes to the bundle definition, package manifests, or package layers cause it to fail.

6. **Publish, pull, and deploy the signed artifact**

   Push the signed artifact to an OCI registry. Its signature evidence is carried with the bundle:

   ```bash
   CLI_FEATURES=NextMode=true uds bundle push \
     ./my-bundle/uds-bundle-my-app-amd64-1.0.0.tar.zst \
     oci://registry.example.com/my-org/my-app:1.0.0
   ```

   Pull it with the same verification policy:

   ```bash
   mkdir -p ./pulled
   CLI_FEATURES=NextMode=true uds bundle pull \
     oci://registry.example.com/my-org/my-app:1.0.0 \
     --output-dir ./pulled \
     --public-key ./cosign.pub
   ```

   Deploy the OCI reference with signature verification enabled:

   ```bash
   CLI_FEATURES=NextMode=true uds bundle deploy \
     oci://registry.example.com/my-org/my-app:1.0.0 \
     --public-key ./cosign.pub \
     --config ./config.uds.hcl
   ```

   For a keyless artifact, use the identity and issuer flags shown above.

</Steps>

## Verification

Before distributing an artifact, confirm that:

- `uds bundle verify` succeeds with the public key or keyless constraints used by deployers.
- The artifact was signed after its final `bundle.uds.hcl`, defaults, values, and package content were assembled.
- The published OCI reference points to that verified artifact, and deployers have the required key or keyless constraints.

## Related documentation

- [Next mode reference](/cli/reference/next-mode/) - Overview of Next mode behavior and commands.
- [bundle.uds.hcl](#migration-doc-docs-reference-next-bundle-uds-hcl-mdx) - Configure package signature verification for Next-mode bundle packages.
- [Zarf package signing](https://docs.zarf.dev/ref/package-signing/) - Zarf's package signing and verification reference.
