# 1. Bundles Next Spec

Date: 2026-01-22
Updated: 2026-08-27

## Changelog

| Date | Change |
|------|--------|
| 2026-01-22 | Initial ADR - HCL bundle definition, `bundle.uds.hcl` and `config.uds.hcl` schemas, OCI layout, custom media types |
| 2026-05-05 | CLI-136: the `variables` block accepts list, set, and tuple values in addition to scalars and objects. Layer overrides replace collections wholesale (Helm convention). |
| 2026-08-27 | CLI-318: replace "Better Bundles" with "Bundles Next". |

## Status

Accepted

## Context

UDS bundles are defined using `uds-bundle.yaml`, which has structural and operational limitations:

- **YAML limitations**: No conditionals, composition, reuse, or validation support for complex configuration logic.
- **Redundant overrides**: Bespoke `overrides` mechanisms for namespaces and value injection duplicate features now native in Zarf.
- **OCI indirection**: Bundle manifests use a generic blob requiring extra resolution to locate Zarf packages, making bundles harder to inspect and causing pruning issues in registries. No unique media type for bundle identification.
- **Tofu divergence**: YAML-based bundles create confusion alongside the HCL-based UDS Tofu Provider workflow.

## Decision

### Bundle Definition: HCL

Transition from YAML to [HCL2](https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md) (plain HCL, not Tofu). Eliminate bespoke overrides in favor of upstream Zarf features.

#### `bundle.uds.hcl`

```hcl
uds {
  bundle_api_version   = "uds.dev/v1alpha1"
}

locals {
  repo    = "ghcr.io/defenseunicorns/packages/uds"
  version = "0.59.1-upstream"

  pkgs = {
    base       = "core-base"
    logging    = "core-logging"
  }
}

metadata {
  name        = "uds-core"
  description = "UDS Core bundle (base + logging) plus a local podinfo for smoke testing"
  version     = "0.1.0"
}

package "core_base" {
  source     = "oci://${local.repo}/${local.pkgs.base}:${local.version}"
}

package "core_logging" {
  source     = "oci://${local.repo}/${local.pkgs.logging}:${local.version}"
  depends_on = [package.core_base]
}

# Local Zarf package tarball with Zarf-native configuration inputs
package "podinfo_local" {
  source     = "/path/to/zarf-package-uds-podinfo-amd64-0.1.0.tar.zst"
  depends_on = [package.core_base]

  # Passed directly to Zarf's native namespace handling (single-namespace packages only)
  namespace = "podinfo"

  # Values passed to Zarf as package values; keys must match the Zarf package's value interface
  values_files = [
    "./values/podinfo-base.yaml",
    "./values/podinfo-env.yaml",
  ]
}
```

**Block definitions:**

- **`uds`**: Tooling/schema constraints (e.g., `bundle_api_version`). Fail fast on unsatisfied constraints.
- **`metadata`**: Bundle identity (`name`, `description`, `version`) for display, publishing, and OCI annotations.
- **`package "<id>"`**: Uniquely named artifact entry with `source` (OCI or local). The ID is used for `depends_on` references and need not match the upstream package name.
- **`optional_components`**: Optional Zarf components to enable. Validated at create time; unselected components are excluded from the artifact. Not modifiable at deploy time.
- **`depends_on`**: Ordering intent between packages. Implementation is left to the deployer (DAG, sequential, etc.).
- **`locals`**: Native HCL construct for centralizing reusable values. No external execution dependency required.
- **`namespace` / `values_files`**: Direct passthrough to Zarf's native configuration. Seals the configuration interface to what the bundle author configured.

#### Values File Templating

Values files are rendered at deploy time by Go's standard library `text/template`. Authors get `range`, `if`, `with`, `index`, `printf`, dot-access, pipe, and whitespace-trim markers (`{{- ... -}}`).

Scalar substitution:

```yaml
app:
  name: douginfo
  replicas: 2
database:
  host: {{ .vars.podinfo.dbHost }}
  port: 5432
```

Collection rendering with `range`:

```yaml
gateway:
  service:
    ports:
{{- range .vars.tenantPorts }}
      - name: {{ .name }}
        port: {{ .port }}
        protocol: {{ .protocol }}
{{- end }}
```

Conditional + nested-map traversal:

```yaml
keycloak:
{{- with .vars.keycloak }}
  extraVolumes:
  {{- range .extraVolumes }}
    - name: {{ .name }}
      secret:
        secretName: {{ .secret.secretName }}
  {{- end }}
{{- end }}
```

Variables can include build-time defaults, overridable at deploy time or via a "reconfigure" operation that produces a new bundle sharing most OCI layers.

#### `config.uds.hcl` (Deploy-Time Configuration)

```hcl
# Note: this is an early illustrative example. See ADR-0006 for the authoritative option set.
options {
  architecture    = "arm64"
  plain_http      = false
  skip_tls_verify = false
  uds_cache       = "/tmp/uds-cache"
  tmp_dir         = "/tmp/tmp_dir"
  concurrency     = 3
}

# Variables template values files AND match Zarf variables
variables = {
  domain = "uds.dev"

  podinfo = {
    dbHost = "db.example.com"
  }
}
```

Variable values may be scalars (string, number, bool), nested objects, lists, sets, or tuples. Example covering the new collection types:

```hcl
variables = {
  tenantPorts = [
    { name = "tcp-foo", port = 8080, protocol = "TCP" },
    { name = "tcp-bar", port = 9090, protocol = "TCP" },
  ]

  bucketTags = ["prod", "logs", "primary"]

  keycloak = {
    extraVolumes = [
      { name = "tls-certs", secret = { secretName = "kc-tls" } },
    ]
  }
}
```

Behaviour:

- **Accepted value types**: string, number, bool, object, list, set, tuple.
  - `cty.Map` is not supported; HCL literals always produce `cty.Object`, never `cty.Map`.
- **Set ordering** is stable for a given value but not user-controlled; prefer lists when order matters.
- **Overlay merge**: nested objects deep-merge across the defaults → config layers; everything else (scalars, lists, sets, tuples) is replaced wholesale by the higher-precedence layer.
- **Zarf passthrough**: top-level scalar variables are passed through to Zarf deploy-time `###ZARF_VAR_*###` substitutions. Non-scalar values are only supported in `values_files` templates.

### OCI Structure

On-disk (unpacked tar) layout following the [OCI image layout format](https://github.com/opencontainers/image-spec/blob/main/image-layout.md):

```
bundle.uds.hcl
values/
  podinfo_local/
    0.yaml
    1.yaml
oci/
  oci-layout
  index.json
  blobs/
    sha256/
      <manifest/config/layer blobs for all packages>
```

- `values/<package_id>/<number>.yaml`: Values files in order specified in `bundle.uds.hcl`.
- `oci/index.json`: Entrypoint listing included packages:

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:6032b1d1029d00932fd44e3a4ac93a5ee62f0732d47b022e821c8688fc6c3c55",
      "size": 3362,
      "annotations": {
        "org.opencontainers.image.ref.name": "ghcr.io/defenseunicorns/packages/uds/core-base:0.59.1-upstream"
      }
    }
  ]
}
```

#### OCI Manifest

Custom media types for bundle identification:

- **Config**: `application/vnd.uds-bundle.config.v1+json`
- **Layers**: `application/vnd.uds-bundle.layer.v1.blob`

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.uds-bundle.config.v1+json",
    "digest": "sha256:CONFIG_DIGEST",
    "size": 420
  },
  "layers": [
    {
      "mediaType": "application/vnd.uds-bundle.layer.v1.blob",
      "digest": "sha256:HCL_DIGEST",
      "size": 2610,
      "annotations": {
        "org.opencontainers.image.title": "bundle.uds.hcl"
      }
    },
    {
      "mediaType": "application/vnd.uds-bundle.layer.v1.blob",
      "digest": "sha256:VALUES_0_DIGEST",
      "size": 2615,
      "annotations": {
        "org.opencontainers.image.title": "values/podinfo_local/0.yaml"
      }
    },
    {
      "mediaType": "application/vnd.uds-bundle.layer.v1.blob",
      "digest": "sha256:PKG_INDEX_DIGEST",
      "size": 1800,
      "annotations": {
        "org.opencontainers.image.title": "oci/index.json"
      }
    }
  ],
  "annotations": {
    "org.opencontainers.image.title": "uds-core",
    "org.opencontainers.image.description": "UDS Core bundle ..."
  }
}
```

Key properties:
- Remote OCI packages are ingested by resolving manifests and copying blobs into the internal `oci/` layout (no unpacking needed).
- Local Zarf tarballs are imported into the same OCI layout for uniform representation.
- Blob deduplication via content-addressing; registries reuse blobs via cross-repo blob mounts.
- The OCI manifest structure is part of the CLI's API surface and must be protected from breaking changes.

## Consequences

### Positive

- **Stronger configuration language**: HCL provides conditionals, composition, locals, and validation not possible in YAML.
- **Tofu alignment**: Shared HCL syntax enables smoother interop with the UDS Tofu Provider.
- **Simplified overrides**: Delegating to Zarf's native namespace/values features removes duplicated logic.
- **Inspectable OCI artifacts**: All package layers are explicit in the manifest with a unique mediatype, eliminating indirection and registry pruning issues.
- **Blob reuse**: Content-addressed OCI layout enables efficient storage and transfer.

### Negative

- **No backward compatibility**: Existing `uds-bundle.yaml` bundles must be migrated (migration tool planned separately).
- **HCL familiarity**: Less common than YAML among platform engineers, though arguably second-most familiar.
- **No JSON schema**: HCL doesn't support JSON schema for IDE validation (language server extensions possible later).

## Non-Goals

- Air-gapped OpenTofu execution (separate design doc).
- New bundle-level environment-specific logic beyond upstream Zarf features.
- "Single bundle for entire environment" approach (better suited to Tofu-based deployment).
- State management (handled by Tofu provider and/or Fleet/remote agent).

## Alternatives Considered

- **Full OpenTofu as bundle engine**: Tightly couples to Tofu workflow, reduces portability, diverges from Fleet.
- **Retain YAML format**: More familiar but limited for advanced configuration, diverges from Tofu provider model.
- **Keep bespoke overrides**: Appealing given current adoption, but diverges from upstream Zarf and increases maintenance burden across deployers.
- **OCI index referencing package manifests**: Preserves graph semantics but introduces indirection and weakens offline portability.
- **Packages as opaque tarball layers**: Simpler assembly but prevents blob-level reuse.
- **Per-package OCI layout directories**: Adds isolation but duplicates blobs and metadata.

## References

- Original design doc: "Better Bundles" by Micah Nagel (2026-01-22)
- [HCL2 Syntax Spec](https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md)
- [OCI Image Layout Spec](https://github.com/opencontainers/image-spec/blob/main/image-layout.md)
