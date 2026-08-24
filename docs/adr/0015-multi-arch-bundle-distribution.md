# 15. Multi-Architecture Bundle Distribution via Root Index

Date: 2026-07-09

## Status

Accepted

## Context

A UDS bundle is created for exactly one architecture (`uds-bundle-<name>-<arch>-<ver>.tar.zst`),
and its OCI index holds one manifest entry **per member package** plus the bundle definition
manifest (ADR-0007). Until now each member package entry carried a `platform` field, so a
four-package amd64 bundle produced an index with four entries all declaring `linux/amd64`.

This overloads the OCI `platform` mechanism and creates a real distribution flaw:

- **`platform` cannot key anything inside the bundle index.** Entries are disambiguated by the
  `org.opencontainers.image.ref.name` annotation (packages) and `artifactType` (definition), so
  the per-entry platform is redundant noise - and misleading to any generic tool that performs
  platform selection on the index, which would silently grab the first member package.
- **Same-tag multi-arch is impossible.** Zarf packages achieve multi-arch by making `platform`
  the unique key in the tag's index and merging per-arch entries on publish
  (`zoci.UpdateIndex`). The bundle index has already spent its entries on member packages, so
  there is no slot for a second architecture: merging would produce duplicate `ref.name` keys
  with conflicting digests and an ambiguous second definition manifest.
- **Push silently clobbers.** `PushBundle` pushes the local index verbatim to the tag; pushing
  the arm64 build of a bundle over the amd64 tag replaces it with no warning.

### Alternatives Considered

#### A. Arch-suffixed tags by convention (status quo)

Publish `:0.1.0-amd64` and `:0.1.0-arm64` as separate tags.

- **Pro**: No format change.
- **Con**: Convention-only; nothing prevents the silent same-tag clobber.
- **Con**: Worse than Zarf package parity; consumers must know the suffix scheme.

#### B. Merge per-arch entries into the existing bundle index

Extend the single index with entries for both architectures.

- **Con**: Structurally impossible without breaking the format: `ref.name` keys collide across
  architectures, and consumers cannot associate a definition manifest with an architecture.

#### C. Root index of per-arch bundle indexes (chosen)

Add one level of indirection - exactly how Docker/Zarf represent multi-arch - with the
"index of members" pushed down one level. The tag resolves to a **root (parent) index** whose
entries are platform-keyed and point at **child indexes**, each child being the canonical
single-arch bundle.

- **Pro**: `platform` is a unique key again at the level where platform resolution happens.
- **Pro**: Publish becomes a well-defined merge (fetch root at tag, upsert this arch's entry).
- **Pro**: The child index - the bytes stored as `index.json` inside the `.tar.zst` - remains
  the single-arch bundle artifact and its digest remains the bundle's canonical identity.
  Digest-pinned references (`repo@sha256:<child>`) address one architecture directly.
- **Pro**: Nested indexes are spec-legal (an index entry's media type may be
  `application/vnd.oci.image.index.v1+json`) and supported by mainstream registries and ORAS.
- **Con**: Readers must resolve one extra level when given a tag.
- **Con**: A naive copy of the root fetches every architecture; pull must platform-select
  before copying.  Because this is what Zarf does though this is largely handled where Zarf
  packages exist today.

## Decision

### Child index (the canonical bundle)

The bundle's OCI index - written as `index.json` in the layout and inside the `.tar.zst` - is
the **canonical single-architecture bundle artifact**:

- It declares a top-level `artifactType: application/vnd.defenseunicorns.uds.bundle.v1`
  (OCI image-spec 1.1 allows `artifactType` on an index and this passes through in OCI 1.0
  registries). This is the sole mechanism for identifying an index as a UDS bundle.
- Member package entries **no longer carry `platform`**. Entries are identified by
  `ref.name` annotation (packages) and `artifactType` (definition manifest), as before.
- The index records its architecture in a top-level annotation
  `uds.dev/architecture: <arch>`, keeping the `.tar.zst` self-describing and letting push
  populate the root index's platform field without parsing member content.
- The index is **deterministic**: `manifests` entries are sorted by digest (making the digest
  independent of HCL package declaration order), `schemaVersion`/`mediaType`/`artifactType`
  are fixed, and annotations are fixed-form. Combined with the definition manifest's pinned
  `created` timestamp (ADR-0007), identical inputs always produce an identical child index
  digest, which is the invariant consumers (e.g. UDS Remote Agent) use to track bundles.

### Root index (the tag)

A published bundle tag resolves to a plain OCI image index (no `artifactType` of its own - it
is a platform router, like a manifest list) whose entries are:

```json
{
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "artifactType": "application/vnd.defenseunicorns.uds.bundle.v1",
  "digest": "<child index digest>",
  "size": ...,
  "platform": { "architecture": "<arch>", "os": "multi" }
}
```

`os: "multi"` matches the Zarf package convention. Entries are sorted by architecture for
determinism.

- **Push** resolves the target tag; when the existing content is a root index it upserts this
  architecture's entry and preserves the others (last-write-wins on concurrent pushes, same as
  Zarf). Any other existing content at the tag is replaced.
- **Pull** resolves the tag to the root, selects the entry for the requested architecture
  (default: runtime architecture), and copies **only that child's graph**. The child index
  bytes are written verbatim as `index.json`, so pulled-then-repushed artifacts round-trip
  byte-identically. Pulling a digest-pinned child reference skips the root level entirely.

### Identification

After platform resolution (or when resolving any reference), a consumer identifies the
artifact from descriptors and, at most, one fetch:

- media type `application/vnd.oci.image.manifest.v1+json` → a Zarf package manifest;
- media type `application/vnd.oci.image.index.v1+json` with artifactType
  `application/vnd.defenseunicorns.uds.bundle.v1` (on the descriptor, or on the fetched
  index itself) → a UDS bundle (child);
- an index whose platform-keyed entries carry the bundle artifactType → a bundle root.

### Compatibility

None. UDS CLI Next is pre-release with no published bundles in use; this is a clean break.
Bundles created before this ADR must be re-created. Readers do not detect or convert the old
shape (per-entry platform, no index artifactType).

## Consequences

### Positive

- Multi-arch bundles publish under one tag with Zarf-parity merge semantics; the silent
  same-tag clobber is gone.
- The child index digest is a stable, deterministic identity for a single-arch bundle,
  suitable for digest-pinned deployment and downstream tracking.
- Identification no longer depends on scanning index entries for the definition manifest;
  the artifact self-describes at the descriptor level.

### Negative

- Readers resolve one extra level for tag references and must platform-select before copying
  to avoid downloading every architecture.
- Very strict OCI 1.0-era registries could reject pushes carrying unknown index fields;
  acceptable for the registries UDS targets.

### Neutral

- Concurrent multi-arch publishes to one tag are last-write-wins unless a registry-level
  conditional update is added later (same exposure as Zarf's `UpdateIndex`).

## Registry Compatibility

This format stores the child indexes and member manifests untagged - only the root index is
tagged - so it depends on registry garbage collection following index references rather than
treating "untagged" as "deletable". A survey of GC implementations (2026-07-09):

| Registry | GC verdict | Mechanism |
|----------|------------|-----------|
| distribution/distribution v3 (incl. UDS Remote Agent's embedded registry) | Safe | `markManifestReferences` recurses through index children; `unmarkReferencedManifest` rescues untagged children of a tagged index. |
| Harbor v2 | Safe | DB-backed `artifact_reference` graph; untagged children are excluded from GC candidate listing, delete refuses referenced artifacts, and root deletion cascades. |
| GitLab container registry (metadata database) | Safe | `manifest_references` rows are written for all index children regardless of media type; `IsDangling` honors them. |
| GitLab container registry (filesystem, offline GC with `--delete-untagged`) | **Broken** | The compat media-type classifier omits the OCI index type and the rescue walk is single-level, so nested children are deleted and their blobs swept. |

### GitLab is out of scope for now

GitLab cannot host bundles today regardless of the GC result above, because each of its two
operating modes fails for a different reason:

- **With the metadata database** (GitLab.com and current self-managed): GC is safe, but every
  media type is validated against a seeded `media_types` table on push. The bundle media
  types (`application/vnd.defenseunicorns.uds.bundle.*`) are unknown, so the push is rejected
  with `MANIFEST_INVALID` unless the operator enables the off-by-default
  `REGISTRY_FF_DYNAMIC_MEDIA_TYPES` feature flag.
- **Without the metadata database** (filesystem mode): the push is accepted (no media-type
  validation on that code path), but offline GC with `--delete-untagged` destroys the
  bundle's nested children.

Since neither mode works out of the box, GitLab support is **deferred**. A fix for the
offline GC (recognize the OCI index media type in the compat classifier + walk referenced
manifests breadth-first) has been prepared for upstream submission.

**If GitLab support becomes a requirement**, in order of preference:

1. Metadata-database mode with `REGISTRY_FF_DYNAMIC_MEDIA_TYPES` enabled - no format change.
2. Filesystem mode once the upstream GC fix lands - no format change.
3. Filesystem mode on unfixed registries - implement the digest-derived child tags below.

### Deferred mitigation: digest-derived child tags

If bundles must survive a GC that cannot follow nested indexes, push can additionally tag
each child index with `sha256-<hex of child digest>` (the cosign convention - `:` and `@`
are not legal in tag names). This is designed but intentionally **not implemented**.

It works because a tagged child is walked as a first-class manifest list in the GC's tagged
pass, and its member references are plain OCI image manifests - a media type even GitLab's
unfixed classifier recognizes - so members and their blobs are rescued. The root's
index-typed reference to the child is still misclassified as a blob, but that only marks the
child's bytes, which is harmless once the child is independently tagged.

Digest-derived tags were chosen over named arch tags (`<tag>-<arch>`) because they are:

- **Collision-free** - content-addressed namespace; cannot clash with a user's own
  `1.0.0-amd64` tag or trip version-pattern retention/immutability rules.
- **Recognizable as machinery** - cosign/notation already populate registries with
  `sha256-<hex>.sig` tags, so UIs, retention policies, and operators treat the shape as
  infrastructure rather than a release.
- **Self-describing** - the tag *is* the child digest, so consumers can go tag → digest
  without a fetch.

Implementation notes for whoever picks this up:

- Sha tags are immutable by nature: a same-arch re-push creates a new sha tag while the old
  one keeps the superseded child - and all its blobs - pinned forever. The root-merge step in
  push must therefore delete the stale `sha256-<oldhex>` tag when it replaces an arch entry
  (it already knows the old digest from the existing root entry).
- Tag-only DELETE is optional in the distribution spec. distribution v3, GitLab, and Harbor
  support it; where unsupported, make the untag best-effort - the cost degrades to one
  superseded bundle lingering per arch, not unbounded growth.
- Pull and downstream consumers need no changes; digest-pinned references behave as today.
