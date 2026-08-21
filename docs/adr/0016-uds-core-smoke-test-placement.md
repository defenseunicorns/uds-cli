# 16. UDS Core Smoke Test Placement

Date: 2026-07-28

## Status

Accepted

## Context

ADR-0014 established `cluster_integration` under `tests/cluster/` as the future home for on-cluster integration coverage. The first real UDS Core smoke suite surfaced two practical issues with that shape:

1. `cluster_integration` describes the infrastructure, not the intent of the suite.
2. `tests/cluster/` split closely related bundle integration coverage away from the rest of the bundle integration tests, even though the smoke suite is specifically about layered UDS Core bundle behavior.

Review feedback on CLI-170 also pushed toward a structure that keeps UDS Core smoke coverage distinct, but not isolated in its own standalone workflow or generic cluster-only bucket.

## Decision

UDS Core smoke coverage will:

- live under `tests/integration/<mirrored-pkg-path>/`
- use a dedicated build tag: `uds_core_smoke`
- keep a dedicated task entrypoint: `maru run test:uds-core-smoke`
- run from the main `test.yaml` workflow as a separate matrix entry instead of a standalone workflow

This keeps the suite clearly labeled as smoke coverage, colocates it with the related bundle integration tests, and preserves a distinct CI/runtime path for the expensive live-cluster execution.

## Consequences

- UDS Core smoke tests are no longer treated as the first implementation of the generic `cluster_integration` tier from ADR-0014.
- ADR-0014 remains the baseline for future on-cluster coverage, but UDS Core smoke tests are an explicit exception with their own placement and tag.
- Test guidance and CI/task naming should refer to `uds_core_smoke` and `test:uds-core-smoke` for this suite.
- This decision supersedes the `tests/cluster/` plus `cluster_integration` placement rule from ADR-0014 for UDS Core smoke coverage only.
