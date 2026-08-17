# 10. Legacy and Next Coexistence

Date: 13 August 2026

## Status

Accepted

## Context

UDS CLI Legacy and UDS CLI Next must coexist in one Go module and one `uds` binary. Legacy is the existing CLI and library surface, while Next introduces the canonical package layout. The repository therefore reserves canonical `internal` and `pkg` paths for Next and namespaces Legacy implementation and library packages under `internal/legacy` and `pkg/legacy`.

## Decision

Use the following durable source to destination mapping:

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

The `NextMode` feature switch selects the Next command tree when available. Legacy remains the default, and the current release keeps Next unavailable.

## Consequences

Existing Legacy commands and library imports remain available under their Legacy namespaces, while future Next code can use the canonical paths without conflicting with them.
