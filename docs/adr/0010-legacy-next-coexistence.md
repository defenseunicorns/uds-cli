# 10. Legacy and Next Coexistence

Date: 13 August 2026

## Status

Accepted

## Context

UDS CLI Legacy and UDS CLI Next must coexist in one Go module and one `uds` binary. Legacy remains the default command tree, while canonical paths are reserved for Next.

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
