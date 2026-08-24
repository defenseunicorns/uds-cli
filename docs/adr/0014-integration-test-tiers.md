# 14. Integration Test Tiers

Date: 2026-06-02

## Status

Accepted

## Context

Unit tests cover isolated logic but cannot verify that the CLI binary, the deploy library, and a live cluster all wire together correctly. Three distinct integration tiers are needed, each with a different scope, cost, and infrastructure requirement.

## Decision

Three tiers sit above unit tests:

| Tier | Build tag | Location | What it drives | Infrastructure |
|------|-----------|----------|----------------|----------------|
| **Library integration** | `library` | `tests/library/` | `pkg/bundle` interfaces directly (`Deployer`, `ZarfDeployer`, hooks, public fields `ClusterDeployFn`, `PackageDeployFn`) | None |
| **CLI integration** | `integration` | `tests/integration/<mirrored-pkg-path>/` | Cobra root via `SetArgs` / `Execute` | None (binary only) |
| **On-cluster integration** | `cluster_integration` | `tests/cluster/` | Full deploy against a running cluster with an init package installed | k3d or real cluster |

**Library integration** - drives `pkg/bundle` interfaces directly without a binary, cluster, or registry. Tests hook wiring, layout mutation, and the public seams (`ClusterDeployFn`, `PackageDeployFn`). Fast; run in CI on every PR (`maru run test:integration-library`).

**CLI integration** - drives the cobra root, verifying flag wiring, config resolution, and the CLI → library boundary. Does not re-test business logic already covered by library tests. Fast; run in CI on every PR (`maru run test:integration`).

**On-cluster integration** - exercises full end-to-end deploy against a live cluster. Requires an init package installed before the suite runs. Slow and costly; run on a separate CI gate (`maru run test:integration-cluster`). Not yet implemented; tracked for a future milestone.

## Consequences

- New `library` tag and `maru run test:integration-library` task added in this PR.
- `cluster_integration` tag and task are reserved but not yet implemented.
- The three-tier naming is reflected in `.agents/skills/testing/references/TESTING.md` and enforced by build tags.
- On-cluster tests must not be run locally without explicit user approval (see project memory).
