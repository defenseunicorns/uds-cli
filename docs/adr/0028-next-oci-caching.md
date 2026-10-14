# 28. Next OCI caching

Date: 2026-10-11

## Status

Proposed.

This supersedes the proposed shared package cache in section 6 of
[ADR-0009](0009-bundle-deploy-from-artifact.md) for remote package retrieval.
Next uses a shared OCI blob store rather than a cache of extracted package
directories. The configuration interface in
[ADR-0006](0006-opinionated-cli-settings.md) remains in force. Legacy caching
is unchanged.

## Context

Next creates bundles by fetching remote Zarf packages into temporary package
layouts, then copying their selected content into the bundle layout. Those
workspaces are discarded after the artifact is assembled. Subsequent creates
download the same content again.

Zarf provides cache-aware remote package APIs and an OCI cache wrapper. Using
these components keeps package behavior upstream and allows bundle operations
to reuse the same content-addressed store. UDS owns configuration, cache
location, command integration, cleanup, and tests.

## Decision

### Cache location and ownership

Store Next's OCI cache under `<uds-cache>/bundle-oci`. Remote Zarf package
retrieval and bundle remote reads share this content-addressed store through
upstream caching components. Upstream owns the cache's internal layout and
storage mechanics.

Keep this cache separate from Legacy's `<uds-cache>/layers` and Zarf's default
`<uds-cache>/images`. Do not migrate or seed those caches.

The CLI resolves configuration in this order: the `~/.uds-cache` default,
`options.uds_cache` in `config.uds.hcl`, then an explicit `--uds-cache` flag.
Create the cache on demand when constructing a remote read source. Local
package sources do not open the remote cache. Public library callers provide
`ConfigOptions.CacheDir`; an empty value leaves caching disabled.

### Remote reads

For create, configure the Zarf remote before package manifest fetching, layer
selection, and `PullPackage`. Keep tag resolution and digest pinning intact:
cache reuse must not prevent checking which content a registry tag currently
identifies. Retain component filtering and package signature verification.
Cached bytes do not establish package or bundle authenticity.

The shared package-source adapter also serves source-directory deploys and
package metadata reads. Those callers use the same cache when configured.
Keep package staging and the final bundle layout operation-owned; the cache
does not replace either layout.

For bundle pull, retain one cache-aware source through bundle index resolution,
signature evidence fetching, and copying the selected architecture's graph into
the operation-owned layout. Tag resolution and signature discovery remain
registry operations. Cache reuse does not make pulls offline or bypass bundle
signature verification. Preserve the canonical index bytes and signature
evidence in the output archive independently of the cache.

Inspect and OCI-source remove use the same cache-aware source for bundle
definitions, package metadata, and signature evidence. Metadata-only reads do
not download package payloads. Inspect with a verification policy retains its
existing full-content pull and verification path. Local archive metadata reads
do not open the remote cache. The shared metadata helper also enables caching
for deploy's preliminary unsigned-bundle check.

Subsequent changes will configure the remaining OCI-source deploy reads
through `internal/oci`. Local graph copies may
continue using ORAS; remote reads must consume a cache-aware source.
Audit the upstream helpers actually used rather than assuming every helper
honors its remote's cache.

### Upstream behavior and validation

UDS reuses upstream caching behavior and does not currently guarantee safe
concurrent cache population. Concurrent operations sharing a cache may fail or
corrupt its index. Resolving these limitations remains follow-up work; the
current integration must not be presented as concurrency-safe. Test concurrent
UDS operations and document unmet integrity or concurrency requirements for
upstream resolution. Do not add a separate UDS publication algorithm, private
cache, seeding workflow, or copy-back step.

For remote package retrieval, retry once without caching after an identifiable
cache filesystem failure. Discard partial downloads before retrying, retain any
resolved package digest, and preserve component selection and signature
verification. Do not alter the caller's configuration or repair the shared cache.
Warn when falling back; if the retry fails, preserve both errors.

Do not retry cancellation, package verification failures, or unrelated errors.
Upstream does not provide a distinct cache-error type, so fallback is limited to
filesystem errors identifying the configured cache. Other errors, including
unclassified corrupt-index errors, still propagate. This fallback does not make
concurrent cache population safe. Bundle pull fallback remains separate work.

Integration tests must demonstrate initial population, reuse across independent
creates, unchanged package verification, and the resulting artifact's ability
to supply package content. Add cross-operation reuse and concurrency coverage
as the remaining integrations and dependency updates land.

### Cleanup and diagnostics

A subsequent change will add `uds tools clear-cache` to remove content under
`<uds-cache>/bundle-oci` only, following configuration precedence and Next's
`--prompt` convention. Concurrent cleanup is unsupported; the user chooses
when to run it. Automatic eviction is outside this decision.

Provide debug diagnostics identifying the configured cache. Hit/miss counters
are not required.

## Consequences

Repeated remote package retrieval can reuse cached blobs while retaining Zarf's
package selection and verification APIs. Package content remains available in
the produced bundle independently of the cache.

The shared store consumes persistent disk space in addition to temporary
workspaces and bundle archives. Integrity, concurrency, and failure behavior
depend on upstream components; requirements beyond their behavior need upstream
resolution and UDS integration tests.
