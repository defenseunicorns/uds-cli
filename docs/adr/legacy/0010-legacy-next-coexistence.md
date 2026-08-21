# 10. Legacy and Next Coexistence

Date: 13 August 2026

## Status

Accepted

## Context

UDS CLI Legacy and UDS CLI Next must coexist in one Go module and one `uds` binary. Legacy is the existing CLI and library surface, while Next introduces the canonical package layout. Next ADRs live under `docs/adr`, and Legacy ADRs are retained under `docs/adr/legacy`. The repository therefore reserves canonical `internal` and `pkg` paths for Next and namespaces Legacy implementation and library packages under `internal/legacy` and `pkg/legacy`.

## Decision

- Legacy code resides under `internal/legacy` and `pkg/legacy` for isolation and future removal.
- Next code uses the canonical `internal` and `pkg` paths, so new development does not depend on Legacy paths.
- `NextMode` selects the command tree.
- Legacy mode remains the default during alpha.
- Next mode is an alpha preview and is expected to become the default in beta.
- Legacy mode will be removed after the beta migration window.

The following mapping documents this PR's implementation:

| Source | Destination |
| --- | --- |
| `main.go` | `cmd/uds/main.go` |
| `src/cmd/**` | `internal/legacy/cli/**` |
| `src/config/**` | `pkg/legacy/config/**` |
| `src/types/**` | `pkg/legacy/types/**` |
| `src/pkg/**` | `pkg/legacy/**` |
| `src/test/e2e/**` | `tests/legacy/e2e/**` |
| `src/test/common.go` | `internal/legacy/testutil/testutil.go` |
| `src/test/{bundles,packages,tasks}/**` | `testdata/legacy/{same area}/**` |
| `uds-cli-next/internal/**` | `internal/**` |
| `uds-cli-next/pkg/**` | `pkg/**` |
| `uds-cli-next/tests/**` | `tests/**` |

## Consequences

Existing Legacy commands and library imports remain available under their Legacy namespaces, while future Next code can use the canonical paths without conflicting with them. New product work should target Next packages unless maintainers explicitly scope the change to Legacy compatibility.
