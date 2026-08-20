# 11. Next Test Tiers

Date: 20 August 2026

## Status

Accepted

## Context

UDS CLI now carries both the Legacy CLI and the imported Next implementation. Unit tests cover package logic, but the Next implementation also needs confidence at command, library, cluster, and product-smoke boundaries.

The previous `uds-cli-next` repository used separate test tiers for these boundaries. The combined repository keeps that tiering, but names tasks and CI checks with a `Next` prefix so they are distinct from Legacy E2E coverage.

## Decision

Next test coverage is organized into the following tiers:

| Tier | Task | Build tag | Location | CI check |
| --- | --- | --- | --- | --- |
| Unit | `uds run test:unit` | none | `cmd/...`, `internal/...`, `pkg/...` | `Unit Tests` |
| Integration | `uds run test:next-integration` | `integration` | `tests/integration/...` | `Next / Integration` |
| Integration Library | `uds run test:next-integration-library` | `library` | `tests/library/...` | `Next / Integration Library` |
| Cluster | `uds run test:next-cluster` | `cluster_integration` | `tests/cluster` | `Next / Cluster` |
| UDS Core Smoke | `uds run test:next-smoke-uds-core` | `uds_core_smoke` | `tests/smoke/...` | `Next / Smoke UDS Core` |

The UDS Core smoke tier remains gated by the existing `include_uds_core_smoke` workflow input. It runs in release and nightly flows, not in standard pull request checks.

## Consequences

- Standard pull requests exercise Next unit, integration, library integration, and cluster coverage.
- UDS Core smoke remains available before release without adding normal PR cost.
- New Next tests should use the lowest tier that proves the behavior.
- Legacy E2E test names keep the `Legacy / E2E ...` prefix to make required checks unambiguous during the transition.
