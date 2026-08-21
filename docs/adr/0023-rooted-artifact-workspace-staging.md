# 23. Rooted Artifact Workspace Staging

Date: 2026-08-18

## Status

Accepted

Applies [ADR-0021](0021-filesystem-trust-boundaries-for-commands.md) to
artifact materialization and extracted-package staging, and amends the related
portions of [ADR-0009](0009-bundle-deploy-from-artifact.md). All other ADR-0009
decisions, including digest verification and `PackageLayoutLoader`, remain in
force.

## Context

Artifact deployment reconstructs two kinds of files from OCI blobs:

- bundle definition files (`bundle.uds.hcl`, `defaults.uds.hcl`, and package
  values files); and
- the Zarf package file tree consumed by Zarf during deployment.

OCI layer titles describe relative paths, so ordinary path joins followed by
normal filesystem operations are vulnerable to path traversal and symlink
redirection. Artifact-backed packages can also contain multi-gigabyte component
archives and image layers. Copying every immutable blob into a staging tree can
double temporary disk use and fail on constrained hosts or CI runners.

Artifact OCI layouts may additionally be caller-owned caches or mounts. A cache
may be read-only, quota-limited, or on a filesystem that does not support hard
links, while the configured temporary directory remains usable.

## Decision

### Rooted materialization

Artifact extraction continues to create an operation-owned workspace. Bundle
definition and values layers are materialized through an `os.Root` opened on
that workspace. Layer-title paths must remain beneath the workspace; rooted
directory creation and writes provide the enforcement boundary for symlinks as
well as lexical path traversal.

### Rooted package staging

`ExtractedArtifactPackageLayoutLoader` exposes the parent of its normalized
`OCIDir` as its preferred package staging root. `ZarfDeployer` first creates a
per-package staging directory there. Consequently, the OCI blobs and package
staging tree share one `os.Root`:

```text
<artifact-workspace>/
  oci/blobs/sha256/<blob>       # source
  zarf-pkg-*/<layer-title>      # package staging destination
```

For destinations inside this shared workspace, regular immutable blob layers
are staged with `os.Root.Link`, avoiding a second package-sized allocation.
`images/index.json` is always copied because Zarf updates it during deployment.
If a blob path contains a symlink component, it is copied through the rooted
source and destination rather than linked, so a relative symlink is not
relocated into a different directory context. A failed hard link also falls
back to a rooted copy.

The workspace root is the security boundary for this optimization. It prevents
layer reconstruction from escaping into arbitrary host paths. It does not make
the individual `zarf-pkg-*` directory a separate filesystem jail; a path may
only resolve elsewhere within the operation-owned artifact workspace.

### Caller-owned destinations and cache fallback

The public `PackageLayoutLoader` contract still permits any existing,
caller-owned `dstDir`. If that directory is outside the artifact workspace,
the loader copies layers through a source root for the workspace and a
destination root for `dstDir`; it does not attempt a shared-root hard link.

If creating the preferred colocated staging directory fails, deployment uses
the configured `Config.Options.TmpDir`. If colocated staging later exhausts
storage (`ENOSPC` or `EDQUOT`), deployment removes the partial staging directory
and retries once under `Config.Options.TmpDir`. Permission and source-read
errors are returned directly because they cannot be reliably attributed to the
staging destination. Package validity, manifest, and layout errors do not
trigger a retry.

Rooted copies use short, randomly named, exclusive temporary files in the
destination directory before an atomic rename. This avoids collisions with OCI
titles such as `foo.tmp` and avoids exceeding per-component filename limits.

### Threat model

This decision protects against artifact-provided path traversal and symlink
resolution escaping the artifact workspace into host files. It does not add a
new defense against a concurrent process controlled by the same local user
changing workspace paths between independent filesystem operations; ADR-0009
continues to describe that TOCTOU limitation.

## Consequences

### Positive

- Artifact materialization and normal package staging cannot escape the
  operation-owned workspace into arbitrary host paths.
- Normal artifact deploys retain space-efficient hard links for immutable,
  potentially large OCI layers.
- Read-only, quota-limited, or incompatible cache filesystems can fall back to
  the configured temporary directory.
- Direct library callers retain support for destinations outside the artifact
  workspace.

### Negative

- Package staging includes fallback and retry behavior that must preserve the
  distinction between filesystem-capacity failures and invalid package data.
- Artifact workspaces contain transient `zarf-pkg-*` directories while a
  package deploy is in progress.
- Cross-filesystem and caller-owned destinations copy package bytes and can
  require additional temporary disk space.

## Alternatives Considered

### Always copy OCI layers

Rejected. It provides the same rooted path boundary but duplicates large
immutable package layers and can exhaust temporary storage unnecessarily.

### Use ordinary `os.Link` after lexical validation

Rejected. The destination would be resolved through ordinary path traversal,
which does not preserve the rooted workspace boundary.

### Require all loader destinations to be inside the artifact workspace

Rejected. The public loader contract supports caller-owned destinations and
future cache-backed loaders; those callers must continue to work through a
safe rooted-copy path.

## References

- [ADR-0009 - Bundle Deploy from Artifact](0009-bundle-deploy-from-artifact.md)
- [ADR-0010 - Values File Handling for Bundle Deploy from Artifact](0010-values-file-handling-for-bundle-deploy-from-artifact.md)
- [ADR-0021 - Filesystem Trust Boundaries for Commands](0021-filesystem-trust-boundaries-for-commands.md)
