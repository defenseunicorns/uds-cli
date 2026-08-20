# 8. Bundle Reconfigure Operation

Date: 2026-04-06

## Status

Accepted

## Context

The original Better Bundles design doc introduces the concept of a "reconfigure" operation:

> Variables could (and should) also include "defaults" at build time, providing a way of both documenting available values, and actually setting default configuration that can be changed later. Any defaults can be overridden at either deploy time OR with a targeted "reconfigure" operation that allows swapping in a new set of defaults as a *build* operation. This reconfiguration would result in a new bundle that could be published/deployed, but would share the majority of the underlying OCI layers.

This supports two key personas:

- **Jack** needs a "one-click" deployable artifact - a bundle with sensible defaults baked in, ready to deploy without additional configuration.
- **Jacquline** needs to tailor that same bundle for a different environment - swap in new defaults (e.g., different domains, replica counts, database endpoints) without rebuilding from source.

Today, bundles are created from source (`bundle.uds.hcl` + values files + package references) via `uds bundle create`. If Jacquline receives a built bundle artifact and wants to change its default variables, she must either:

1. Obtain the original source repository and re-run `uds bundle create` with a modified `defaults.uds.hcl` - requiring access to the source and all upstream package registries.
2. Override variables at deploy time via `config.uds.hcl` - which works but does not produce a redistributable artifact. Every subsequent deployer must also know to apply the same overrides.

Neither option enables the intended workflow: take a built bundle, swap its defaults, and produce a new self-contained artifact that anyone can deploy without additional configuration.

### How defaults are stored today

As implemented in [#165](https://github.com/defenseunicorns/uds-cli-next/pull/165) and documented in [ADR-0007](0007-bundle-definition-and-value-files-in-oci-layout.md), bundles are OCI image layouts packaged as `.tar.zst` archives. The bundle definition is stored as an OCI 1.1 artifact manifest (identified by `artifactType: application/vnd.defenseunicorns.uds.bundle.definition.v1`) within the layout's `index.json`. This manifest contains content-addressed layers:

1. `bundle.uds.hcl` - the bundle definition (media type `application/vnd.defenseunicorns.uds.bundle.hcl.v1`, annotation `org.opencontainers.image.title: bundle.uds.hcl`)
2. `defaults.uds.hcl` - optional default variables (same media type, annotation `org.opencontainers.image.title: defaults.uds.hcl`)
3. Values files - per-package YAML files (media type `application/vnd.defenseunicorns.uds.bundle.values.v1+yaml`, annotation `org.opencontainers.image.title: values/<pkg>/<idx>.yaml`)

Package layers (Zarf package manifests, config blobs, image layers) are stored as separate manifests in the same `index.json`, sharing the blob store under `oci/blobs/sha256/`.

### What reconfigure needs to do

Replace (or add) the `defaults.uds.hcl` layer in the bundle definition manifest, producing a new bundle artifact where:

- The `defaults.uds.hcl` blob is replaced with the new content.
- The bundle definition manifest is rewritten (new digest, since a layer changed).
- The `index.json` is updated to reference the new manifest digest.
- All package manifests and their blobs remain untouched.
- The result is output as a new artifact (tarball or OCI tag).

Because OCI content addressing is digest-based, the package blobs are shared by identity - the new artifact contains the same bytes for all package layers. When pushed to an OCI registry, blob deduplication means only the new defaults blob and updated manifest need to be uploaded.

## Decision

### Command: `uds bundle reconfigure`

```
uds bundle reconfigure <source> --defaults <defaults-file> [--suffix <string>]
```

**Arguments:**

- `<source>` (required, positional): Either a local `.tar.zst` bundle tarball or an OCI reference (e.g., `oci://ghcr.io/org/bundle:v1.0.0`). The source type determines the output type - local input produces a local tarball, OCI input produces a new OCI tag.

**Flags:**

- `--defaults <path>` (required): Path to the new `defaults.uds.hcl` file. This file must contain only a `variables` block (same validation as the existing defaults file - the `options` block is not permitted).
- `--suffix <string>` (optional, default: `-reconfigured`): Suffix appended to the output artifact name. For local tarballs, inserted before `.tar.zst`. For OCI references, appended to the source tag.

**Output:**

- **Local source**: The reconfigured bundle is written as a new `.tar.zst` in the current working directory, with the suffix inserted before `.tar.zst`.
- **OCI source**: The reconfigured bundle is pushed to the same registry and repository as the source, with the suffix appended to the source tag.

The input is never modified. Reconfigure fails if the output already exists - a local file with the target filename, or an OCI tag that already resolves in the registry. Use a different `--suffix` or remove the existing artifact first.

**Examples:**

```bash
# Local tarball, default suffix
uds bundle reconfigure ./uds-bundle-core-amd64-0.1.0.tar.zst \
  --defaults ./prod-defaults.uds.hcl
# → writes ./uds-bundle-core-amd64-0.1.0-reconfigured.tar.zst (in cwd)

# Local tarball, custom suffix
uds bundle reconfigure ./uds-bundle-core-amd64-0.1.0.tar.zst \
  --defaults ./prod-defaults.uds.hcl --suffix -il5
# → writes ./uds-bundle-core-amd64-0.1.0-il5.tar.zst (in cwd)

# OCI source, custom suffix
uds bundle reconfigure oci://ghcr.io/org/bundle:v1.0.0 \
  --defaults ./prod-defaults.uds.hcl --suffix -il5
# → pushes to oci://ghcr.io/org/bundle:v1.0.0-il5
```

### Operation semantics

Reconfigure is a **build operation**, not a deploy-time operation. It produces a new, self-contained artifact that can be published, distributed, and deployed independently of the original. The design doc describes this as creating an artifact that acts as a "one-click" deployable for the Jack persona.

The operation is narrow in scope: it replaces the `defaults.uds.hcl` layer and updates the bundle's `name` in `bundle.uds.hcl` to reflect the suffix. It does not modify values files, package contents, or any other bundle metadata. The bundle's structure and package composition are immutable; only the default variable values and the bundle name change.

### Implementation approach: local tarball

The reconfigure operation works entirely within the OCI image layout without re-ingesting packages or contacting registries:

1. **Check output does not exist** - compute the output filename (input basename with suffix inserted before `.tar.zst`) and verify it does not already exist in the current working directory. Fail with a clear error if it does.
2. **Extract** the input `.tar.zst` to a temporary directory.
2. **Parse `index.json`** to locate the bundle definition manifest (the entry with `artifactType: application/vnd.defenseunicorns.uds.bundle.definition.v1`).
3. **Read the bundle definition manifest** from `oci/blobs/sha256/<digest>` and decode its layers.
4. **Validate the new defaults file** - parse it as HCL, confirm it contains only a `variables` block (reject if an `options` block is present), same validation used at create time.
5. **Write the new defaults blob** to the blob store (`oci/blobs/sha256/<new-digest>`).
6. **Update `bundle.uds.hcl`** - extract the HCL layer, parse it, append the suffix to the `name` field in the `metadata` block, and write the updated HCL as a new blob to the blob store.
7. **Rebuild the bundle definition manifest** - replace the `defaults.uds.hcl` layer descriptor with one pointing to the new defaults blob (or insert one if the original bundle had no defaults), and replace the `bundle.uds.hcl` layer descriptor with the updated HCL blob. Preserve values file layers unchanged. Add provenance annotation (see below). Pin the `AnnotationCreated` timestamp to the epoch for reproducibility, matching the create path.
8. **Write the updated manifest** to the blob store and update `index.json` to reference the new manifest digest (replacing the old bundle definition entry).
9. **Clean up unreferenced blobs** - remove old defaults, HCL, and manifest blobs if they are no longer referenced. This mirrors the `gcUnreferencedBlobs` step in the create path.
10. **Repackage** the OCI layout as a new `.tar.zst` in the current working directory, with the suffix applied to the filename.

### Implementation approach: OCI source

When the source is an OCI reference, reconfigure operates surgically against the remote registry without downloading the full bundle. Only the bundle definition manifest and the small defaults blob are transferred - package layers (which make up the vast majority of a bundle's size) are never fetched.

1. **Check output tag does not exist** - compute the target tag (`<original-tag><suffix>`) and attempt to resolve it in the registry. Fail with a clear error if the tag already exists.
2. **Resolve the OCI reference** to get the image index.
2. **Find the bundle definition manifest** entry in the index (by `artifactType: application/vnd.defenseunicorns.uds.bundle.definition.v1`).
3. **Fetch only the bundle definition manifest blob** from the registry.
4. **Validate the new defaults file** - same validation as the local path.
5. **Push the new defaults blob** to the registry.
6. **Fetch, update, and push `bundle.uds.hcl`** - fetch the HCL layer blob, parse it, append the suffix to the `name` field, push the updated HCL as a new blob.
7. **Rebuild the bundle definition manifest** - same layer replacement (defaults + HCL) and provenance annotation as the local path. Push the updated manifest blob to the registry.
8. **Rebuild the image index** - replace the bundle definition manifest entry with the new digest. Push the updated index, tagged with `<original-tag><suffix>`.

This uses existing ORAS capabilities: `repository.Fetch()` for reading individual blobs, `repository.Push()` for writing blobs, and `repository.PushReference()` for tagging the new index. Registry flags (`--plain-http`, `--skip-tls-verify`) are inherited from the bundle parent command.

**Scope limitation**: Cross-layer operations (OCI source → local tarball, or local tarball → OCI push) are not supported in the initial implementation. The output type matches the input type. Cross-layer support can be added in the future if a use case emerges.

### Artifact identity and immutability

Reconfigure produces a **derivative artifact**, not a mutation of the original. The key properties:

- **New digests**: The defaults blob, bundle definition manifest, and index all get new digests. The reconfigured bundle is a distinct OCI artifact.
- **Original untouched**: The input tarball is never modified. For OCI sources, the original tag is untouched; a new tag is created.
- **Updated bundle name**: The `name` field in `bundle.uds.hcl` is updated to include the suffix (e.g., `uds-core` → `uds-core-il5`), so the internal identity matches the external artifact name. The `version`, `description`, and all other HCL content remain unchanged.
- **Package integrity**: All package manifests and blobs retain their original digests. If individual packages carry signatures or attestations, those remain valid.

### Provenance tracking

Reconfigured bundles are identifiable through two complementary mechanisms:

**1. Updated bundle name** - The `name` field in `bundle.uds.hcl` is updated with the suffix, making it immediately visible to anyone reading the HCL or running `uds bundle inspect`. This ensures the internal identity matches the external artifact name, avoiding the confusion that arises when published name/tag and internal metadata diverge (see [zarf-dev/zarf#4609](https://github.com/zarf-dev/zarf/issues/4609) for prior art on this problem).

**2. OCI manifest annotation** - Reconfigure adds an annotation to the new bundle definition manifest:

```
org.defenseunicorns.uds.reconfigured-from: sha256:<source-child-index-digest>
```

This is zero-cost - the manifest is already being rewritten, so the annotation is simply one additional key in the `annotations` map before serialization. The annotation provides:

- A machine-readable link to the original bundle's canonical child index.
- A stable, content-addressed pointer back to the source artifact.

The annotation value is the source child-index digest: the canonical artifact digest of the source bundle. This is the digest of the original `oci/index.json` bytes for a local `.tar.zst`, or the platform-selected child descriptor for an OCI source.

**Chained reconfiguration**: When reconfiguring an already-reconfigured bundle, the annotation always points to the immediate parent. For example, reconfiguring `uds-core-il5` produces an annotation referencing the `uds-core-il5` child-index digest, not the original `uds-core`. Full lineage can be traced by following the chain one hop at a time.

### Signing considerations

The original Better Bundles design doc explicitly defers signing, SBOMs, and checksums to future work:

> This example does not include signatures, sboms, checksums, or other additional artifacts. Anything of this nature would live at the same level as `bundle.uds.hcl`, but is currently not identified/designed explicitly for here.

Reconfigure does not address signing. When bundle signing is introduced in the future, the following will apply:

- A signature over the original bundle's index digest would **not** be valid for a reconfigured bundle, because the index digest changes. This is correct and expected - the artifact has changed.
- A reconfigured bundle would need to be signed independently by whoever performs the reconfiguration.
- This is consistent with how OCI artifact signing works generally: signatures are bound to specific digests, and derivative artifacts require their own signatures.

The reconfigure command should not carry forward, strip, or otherwise manipulate any signature-related artifacts. If signatures are present in a future bundle format, reconfigure should warn that the output is unsigned.

### Alternatives considered

#### A. Allow modifying values files and/or `bundle.uds.hcl`

Extend reconfigure to replace not just defaults but also values files or the bundle definition itself.

- **Pro**: More flexible - could support additional customization scenarios.
- **Con**: Replacing `bundle.uds.hcl` could change package composition, sources, or dependencies - fundamentally altering the bundle in ways that are hard to reason about without re-validation against upstream packages.
- **Con**: Replacing values files has similar risks - incorrect values could break deployment in ways that are only discovered at deploy time.
- **Con**: The design doc specifically describes reconfigure as "swapping in a new set of defaults", not as a general-purpose bundle editor.

**Decision**: Reconfigure only replaces `defaults.uds.hcl`. This keeps the operation safe and predictable. If broader editing capabilities are needed in the future, they should be designed separately with appropriate validation and safeguards.

#### B. In-place mutation of the tarball

Modify the tarball directly rather than producing a new file.

- **Pro**: Simpler output model - no need to manage multiple files.
- **Con**: Violates the principle of immutable artifacts. The original bundle cannot be recovered.
- **Con**: Inconsistent with how other bundle operations work - `create` produces a new file, `pull` produces a new file.
- **Con**: Risk of data loss if the operation fails partway through.

**Decision**: Always produce a new artifact. The original is never modified.

#### C. Re-run `create` with substituted defaults

Instead of manipulating the OCI layout directly, extract the bundle source from the tarball and re-run the create pipeline with a different `defaults.uds.hcl`.

- **Pro**: Reuses existing create code path entirely.
- **Con**: Create requires resolving package sources (OCI references or local paths), which may not be accessible - the whole point of reconfigure is to work with a built artifact without needing upstream access.
- **Con**: Would re-ingest all packages, which is slow and unnecessary since the packages haven't changed.

**Decision**: Operate directly on the OCI layout. This is fast (only the small defaults blob changes) and works fully offline for local tarballs, or with minimal network transfer for OCI sources.

#### D. OCI annotation only, without updating bundle name

Track provenance exclusively via an OCI manifest annotation, leaving `bundle.uds.hcl` completely untouched.

- **Pro**: Only one layer changes (defaults). Simpler implementation.
- **Pro**: The HCL stays exactly as the author wrote it.
- **Con**: Internal name and external name diverge - the bundle's `name` field says `uds-core` but the tarball/tag says `uds-core-il5`. This is the same class of confusion described in [zarf-dev/zarf#4609](https://github.com/zarf-dev/zarf/issues/4609).
- **Con**: `uds bundle inspect` shows the original name, which may mislead operators about what they're deploying.

**Decision**: Update the bundle name in `bundle.uds.hcl` alongside the OCI annotation. The small cost of modifying a second layer is worth the clarity of having internal identity match external naming.

## Consequences

### Positive

- **Enables the design doc's intended workflow**: Jacquline can take a bundle built by someone else, tailor its defaults for her environment, and produce a redistributable artifact - without source repo access or upstream registry connectivity.
- **Fast and offline (local)**: Only a small blob (the defaults file) and the manifest are rewritten. Package layers are untouched. No network access required.
- **Fast and bandwidth-efficient (OCI)**: Only the defaults blob and manifest are transferred. Multi-GB package layers are never fetched. This enables reconfiguration of large bundles without downloading the full artifact.
- **Safe**: The original artifact is never modified. The operation scope is narrow and predictable - only default variable values change.
- **OCI-native layer sharing**: When both the original and reconfigured bundles are in a registry, they share all package blobs. Only the new defaults blob and updated manifests are unique.
- **Traceable**: The updated bundle name makes reconfigured bundles immediately identifiable, and the `org.defenseunicorns.uds.reconfigured-from` annotation provides machine-readable lineage back to the original.
- **Consistent with existing patterns**: Follows the same command pattern (`Complete` / `Validate` / `Run`), uses the same config resolution, and produces the same output format as other bundle commands.

### Negative

- **No overwrite**: Reconfigure refuses to overwrite existing artifacts (local files or OCI tags). Users must remove the existing artifact or choose a different `--suffix`. This is intentionally strict to prevent accidental data loss, but means repeated reconfigures with the same suffix require cleanup first.
- **No validation against package schemas**: Reconfigure does not verify that the new default variables are compatible with the packages in the bundle. Invalid defaults will only surface at deploy time. This is the same behavior as `defaults.uds.hcl` at create time today - variables are free-form maps, not validated against package schemas.
- **OCI reconfigure requires connectivity**: Unlike local reconfigure which is fully offline, OCI reconfigure requires network access and write permissions to the target registry.
- **No cross-layer support**: OCI sources produce OCI output; local sources produce local output. Cross-layer operations (e.g., reconfigure from OCI and output a local tarball) are deferred to future work.

### Neutral

- **Future extensibility**: The implementation approach (locate manifest, replace layer, rewrite) could be generalized to replace other layers if needed. This ADR intentionally does not design for that, but the mechanism does not preclude it.

## References

- [Original design doc: "Better Bundles" by Micah Nagel (2026-01-22)](https://www.notion.so/defense-unicorns/Better-Bundles-2f0e512f24fc80e2b737e3527da85649)
- [ADR-0007: Store Bundle Definition and Value Files in OCI Layout](0007-bundle-definition-and-value-files-in-oci-layout.md)
- [CLI-124: Use defaults file when deploying a bundle](https://linear.app/defense-unicorns/issue/CLI-124) / [PR #165](https://github.com/defenseunicorns/uds-cli-next/pull/165)
- [CLI-111: Provide a way to "reconfigure" a bundle with different default values](https://linear.app/defense-unicorns/issue/CLI-111)
