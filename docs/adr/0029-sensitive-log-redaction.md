# 29. Sensitive standard-log redaction

Date: 2026-11-13

## Status

Accepted. Supersedes ADR-0028's rejection of exact-match replacement for standard logs only. Adds a narrow exception to ADR-0012's prohibition on global logging changes.

## Decision

Around sensitive Zarf backend deployment, replace known sensitive scalar values in Go's standard logger output with `***`. Include nested marked values, ignore empty strings, and match longer values first. Keep existing context-log suppression and error redaction.

Concurrent deployments remain parallel. Each registration installs an immutable filter snapshot. Retain the combined secrets until the last overlapping call finishes, then restore the original writer. Deferred cleanup handles errors and panics.

## Consequences

This is best-effort exact matching: transformed, escaped, or partial secrets can still appear. Matching text in unrelated standard logs is also redacted. Applications must not reconfigure global logging during deployment. Other output paths remain outside this filter.

## Alternatives considered

Suppressing all standard logs loses useful diagnostics. Matching Helm warning text depends on specific messages. Helm/Zarf changes and subprocess isolation are outside scope.
