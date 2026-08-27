# ADR 0024: Immutable unstable release tags

- Status: Accepted
- Date: 2026-08-27

## Context

Unstable releases need to validate the exact source that will be published while remaining distinguishable from stable releases. The former `nightly-unstable` tag was mutable, which made a tag insufficient to identify the source and could cause consumers to receive changing artifacts.

## Decision

The unstable release workflow will:

- Validate the selected branch or commit, release configuration, and reusable release tests before creating a tag.
- Create immutable tags in the form `vX.Y.Z-nightly+YYYYMMDDHHMMSS-XXXXXXXX` for scheduled releases and `vX.Y.Z-adhoc+YYYYMMDDHHMMSS-XXXXXXXX` for manual releases.
- Reject collisions and never move or overwrite an unstable tag.
- Keep unstable tags excluded from the stable release workflow.
- Retain only the newest three nightly prereleases and their tags. Ad hoc prereleases are retained and are not removed by scheduled cleanup.

The mutable `nightly-unstable` release model is replaced by these immutable, source-addressed tags.

## Consequences

Every unstable artifact can be traced to an exact source commit, and validation failures cannot leave an orphaned release tag. Consumers and maintainers must use the generated tag when referring to an unstable release. Nightly cleanup limits scheduled-release storage, while manual ad hoc releases require deliberate lifecycle management.
