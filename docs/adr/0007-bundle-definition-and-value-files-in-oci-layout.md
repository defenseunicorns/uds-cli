# 7. Store Bundle Definition and Value Files in OCI Layout

Date: 2026-03-10

## Status

Accepted

## Context

A UDS bundle is an OCI image layout (an `oci/` directory following the [OCI Image Layout Specification](https://github.com/opencontainers/image-spec/blob/main/image-layout.md)) that aggregates one or more Zarf packages. The bundle is fully described by two inputs the author provides:

1. **`bundle.uds.hcl`** - the bundle HCL file declaring which packages are included, their sources, and any overrides.
2. **Values files** - per-package YAML files supplying Helm value overrides for each package's charts.

These inputs need to travel with the bundle so that:
- The bundle can be reproduced or audited at a later date.
- Pull and deploy operations have the original configuration available without requiring out-of-band access to the source repository.
- Tooling can inspect a bundle's definition from the OCI artifact alone.

### Alternatives Considered

#### A. Loose files alongside the OCI layout (initial implementation with create operation)

Store `bundle.uds.hcl` and values files as plain files next to `oci/` inside the tarball (e.g., `bundle.uds.hcl`, `values/pkg1/0.yaml`).

- **Pro**: Simple; no OCI plumbing required.
- **Con**: Files live outside the OCI layout, so they are invisible to standard OCI tooling (registries, ORAS, `crane`, etc.). Push/pull operations that work on the OCI layout would silently drop them.
- **Con**: Push would have to dynamically convert loose files into OCI layers and construct a manifest on the fly; pull would have to reverse the process - extracting them back out of OCI into loose files. This conversion overhead occurs on every push and pull, whereas storing them as OCI layers from the start means create, push, and pull all operate on the same representation.
- **Con**: No content-addressable storage - files can drift or be replaced without the digest changing.
- **Con**: No standard mechanism to distinguish bundle-definition content from package content when inspecting the index.

#### B. Definition files embedded per package manifest (original OCI design)

The original design had no separate bundle definition manifest in the index. Instead, each package manifest carried bundle-specific content as layers alongside the package's OCI blobs:

- The **bundle HCL** was added as a layer to every package manifest, duplicated across all packages.
- **Values files** were added as layers only to the manifest of the package they applied to.
- The package's OCI content (blobs, index) was stored as additional flat layers within that same manifest rather than as a nested OCI layout.
- A custom config media type (`application/vnd.uds-bundle.config.v1+json`) on each package manifest was used to signal that it was a bundle-augmented manifest.

**Why this was moved away from:**

- **HCL duplication**: Every package manifest carried a full copy of the bundle HCL. In a bundle with N packages, the HCL blob is stored N times - wasteful and a potential consistency hazard if manifests diverge.
- **Package manifests are polluted**: Embedding bundle metadata into package manifests mixes two concerns. A package manifest should describe a Zarf package; bundle-level configuration is not part of that.
- **No single source of truth for the definition**: The bundle definition is spread across all package manifests. Consumers must aggregate layers from every manifest to reconstruct it rather than reading from one place.
- **Flat OCI blobs within layers**: Storing the package's OCI layout as individual flat layers inside the manifest (rather than referencing them via a nested index) is non-standard and makes the package content opaque to OCI tooling.

#### C. Standalone OCI manifest with a single tar/zip layer

Create a standalone OCI manifest in the index (identified by `artifactType`, same as option D) but store all definition files - HCL and values - packed into a single tar or zip blob as one layer, rather than as individual layers.

- **Pro**: Simpler manifest structure - one layer instead of N layers, one blob push/pull instead of N round-trips.
- **Pro**: Definition files are always consumed as a unit anyway; partial fetch of individual files has limited practical value.
- **Con**: The manifest is opaque - to enumerate which files are in the definition, a consumer must download and unpack the blob. With individual layers, the `org.opencontainers.image.title` annotations on each layer make the full file list visible from the manifest JSON alone, without fetching any blobs. This is meaningful for tooling that wants to inspect a bundle's definition (e.g., list which packages have values overrides, or fetch only the HCL for a quick audit) without downloading everything.
- **Con**: Requires a pack/unpack step in both create and pull.
- **Con**: Loses per-file content addressing - two bundles with identical values files cannot deduplicate at the blob level.

#### D. Overload the platform field (Zarf skeleton approach)

Zarf uses `platform.architecture = "skeleton"` on a synthetic manifest to mark a bundle-level artifact in the index.

- **Pro**: Works with OCI tools that understand platform fields.
- **Con**: Semantically incorrect - `platform.architecture` describes CPU architecture, not artifact type. Tooling that filters or routes by architecture will mishandle skeleton entries.
- **Con**: No standard way to encode additional metadata (e.g., media type) without further convention.

#### E. OCI 1.1 artifact manifest with `artifactType` (chosen)

Create an OCI 1.1 artifact manifest whose `artifactType` is `application/vnd.defenseunicorns.uds.bundle.definition.v1`. Store the HCL file and each values file as content-addressed layers within the bundle's OCI layout, each annotated with `org.opencontainers.image.title` to preserve its logical path.

- **Pro**: Fully inside the OCI layout - registries and OCI tooling treat it as a first-class manifest.
- **Pro**: The manifest JSON is self-describing - each file in the definition is visible as a named layer via `org.opencontainers.image.title`, so consumers can enumerate the definition's contents (e.g., which packages have values overrides, or whether a specific file is present) without downloading any blobs.
- **Pro**: Content-addressed layers - any modification to any individual file changes that layer's digest, which in turn changes the manifest digest, making tampering or drift detectable at the file level.
- **Pro**: Per-file deduplication - two bundles sharing identical values files share the same blob in the registry.
- **Pro**: `artifactType` is the OCI-standard mechanism for identifying artifact kind; consumers can filter on it without inventing conventions.
- **Pro**: The manifest's digest is pinned in `index.json`, so the entire bundle definition is referenced from the single index entry point.
- **Pro**: Consistent with how packages are stored - both are OCI manifests in the same index.

## Decision

The bundle definition (HCL file and values files) is stored as an OCI 1.1 artifact manifest within the bundle's OCI image layout. Specifically:

- A single manifest with `artifactType: application/vnd.defenseunicorns.uds.bundle.definition.v1` is created and added to `index.json`.
- The `bundle.uds.hcl` file is stored as the first layer, with media type `application/vnd.defenseunicorns.uds.bundle.hcl.v1` and `org.opencontainers.image.title: bundle.uds.hcl`.
- Each package values file is stored as a subsequent layer, with media type `application/vnd.defenseunicorns.uds.bundle.values.v1` and `org.opencontainers.image.title: values/<package-name>/<index>.yaml`, preserving the logical path of the values file relative to the bundle root.
- The manifest's `AnnotationCreated` timestamp is pinned to the Unix epoch (`1970-01-01T00:00:00Z`) to make the manifest digest reproducible across identical builds.

Consumers that need to locate the bundle definition within an index locate the manifest entry whose `artifactType` matches `application/vnd.defenseunicorns.uds.bundle.definition.v1` and read layers by their `org.opencontainers.image.title` annotation.

## Consequences

### Positive

- **Self-contained bundles**: Push a bundle to any OCI registry and the definition travels with it. Pull it back and the full definition is present without any external lookup.
- **Tamper-evident**: Content-addressed storage means any modification to the HCL or values files changes the layer digest, which in turn changes the manifest digest and the index entry.
- **OCI-native discovery**: Standard OCI tooling (`crane manifest`, ORAS, registry UIs) can list and inspect the bundle definition manifest without UDS-specific knowledge.
- **Semantic correctness**: `artifactType` is the right OCI mechanism for this use case, as opposed to overloading `platform.architecture` or inventing out-of-band conventions.

### Negative

- **OCI 1.1 requirement**: The `artifactType` field on index manifests is an OCI Image Spec 1.1 feature. Registries or tools that only support OCI 1.0 may not surface the field correctly, though they will still store and retrieve the manifest without error.
- **Index contains a non-image manifest**: Consumers that assume every entry in a bundle's `index.json` is a Zarf package manifest must explicitly skip entries whose `artifactType` is `application/vnd.defenseunicorns.uds.bundle.definition.v1`.

### Neutral

- **Values files indexed by position**: Values files are named `values/<package-name>/<index>.yaml` where `<index>` is the zero-based position of the file in the package's `value_files` list. This is deterministic and reproducible but loses the original filename. The original filename is preserved as a comment or can be recovered from the HCL.
