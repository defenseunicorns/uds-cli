# 28. Cache downloaded bundle content

Date: 2026-10-19

## Status

Accepted.

## Context

Legacy caches downloaded image layers so later bundle commands can reuse them.
Next needs the same benefit for package and bundle content. We want to avoid
repeated downloads without maintaining another caching implementation in UDS CLI.

## Decision

We will reuse Zarf's OCI caching logic with a separate, configurable directory
for UDS bundle caching. The cache will hold both Zarf package content and bundle
content, so operations can reuse the same remote sources. We will not cache
extracted packages or command results.

We will make concurrent cache use safe through fixes in Zarf, rather than add
storage or locking logic in UDS CLI. This keeps the solution shared with other
Zarf consumers.

We will keep Next's cache separate from Legacy and Zarf's default caches.
Clearing Next's cache will leave those caches and created bundles untouched.

## Implementation details

- The CLI uses `~/.uds-cache` by default. The `options.uds_cache` HCL setting
  overrides the default; `--uds-cache` takes precedence.
- Go library callers choose the cache location with `ConfigOptions.CacheDir`.
- Next stores cached content under `<uds-cache>/bundle-oci/images`.
- `uds tools clear-cache` clears Next's cache. It supports confirmation with
  `--prompt`.

## Consequences

### Positive

- Repeated references to the same remote sources avoid duplicate downloads.
- We maintain less caching code by reusing Zarf, and other Zarf consumers benefit
  from the same fixes.
- Users can clear Next's cache without affecting Legacy, Zarf, or created bundles.

### Negative

- Cached content uses disk space in addition to temporary files and created bundles.
- Separate caches can store duplicate copies of the same content.
- Cache failures can interrupt commands, and fixes depend on upstream releases.
