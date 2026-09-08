# ADR 0024: Unique snapshot release tags

- Status: Accepted
- Date: 2026-08-27

## Context

Snapshot releases build the current `main` commit while remaining distinguishable from stable releases. The former mutable release tag was insufficient to identify the source and could cause consumers to receive changing artifacts.

## Decision

The snapshot release workflow will:

- Check out and test `main` before creating a tag.
- Create unique tags in the form `vX.Y.Z-snapshot+YYYYMMDDHHMMSS-XXXXXXXX` for scheduled and manual releases.
- Reject collisions and never move or overwrite a snapshot tag.
- Keep snapshot tags excluded from the stable release workflow.
- Retain only the newest three snapshot prereleases while preserving their tags.

The mutable release model is replaced by these unique, source-addressed snapshot tags.

## Consequences

Every snapshot artifact can be traced to an exact `main` commit, and validation failures cannot leave an orphaned release tag. Consumers and maintainers must use the generated tag when referring to a snapshot release. Scheduled cleanup limits snapshot-release artifact storage while preserving tag traceability.
