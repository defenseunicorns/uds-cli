# AGENTS.md

Agent guide for **UDS CLI**, a Go CLI for UDS workflows with Legacy and Next implementations. Format spec: https://agents.md.

Module: `github.com/defenseunicorns/uds-cli`, never change.

## Copyright headers

When modifying a file that already has a copyright header, preserve its starting year. If the current calendar year is later than a single-year header, change it to a range ending in the current year. If the current year is later than the end of an existing range, update only the end year. Otherwise, leave the header unchanged. Do not add headers to files that do not already have one.

## Skills

- [.agents/skills/go-development/SKILL.md](.agents/skills/go-development/SKILL.md), Go coding rules
- [.agents/skills/testing/SKILL.md](.agents/skills/testing/SKILL.md), testing strategy
- [.agents/skills/documentation-authoring/SKILL.md](.agents/skills/documentation-authoring/SKILL.md), documentation authoring rules

## Legacy and Next separation

UDS CLI is migrating from Legacy to Next. Keep these lanes separate.

Legacy:

- Code: `internal/legacy/...`, `pkg/legacy/...`
- Tests and fixtures: `tests/legacy/e2e`, `testdata/legacy`
- ADRs: `docs/adr/legacy/...`

Next:

- Standalone Next entrypoint: `cmd/uds-cli-next`
- Primary built CLI entrypoint: `cmd/uds`, with Next enabled by feature flag
- Private implementation: canonical `internal/...` packages outside `internal/legacy`
- Public packages: `pkg/bundle/...`, `pkg/iostreams`
- Tests: `tests/cli`, `tests/library`, `tests/cluster`
- ADRs: `docs/adr/...`

Rules:

- New feature work targets Next unless a maintainer explicitly scopes it to Legacy.
- Preserve Legacy behavior unless the work intentionally changes Legacy.
- Do not make Legacy packages depend on canonical Next packages.
- Keep Cobra wiring out of business logic. Next Cobra wiring belongs in `internal/cli`.
- Next command handlers return errors through `RunE`; only the executable reports errors and exits.
- Use a fresh Cobra root and `iostreams` for CLI tests. Reserve the primary `cmd/uds` binary for the minimal process checks: Zarf passthrough, Next-mode routing, and process error/exit behavior.
- For Zarf package and OCI implementation choices, follow the Go skill's Zarf and OCI operation ownership guidance.

## Next testing

[ADR-0027](docs/adr/0027-public-api-test-pyramid.md) defines three test layers:
unit, public library, and CLI. Preserve the large colocated unit-test investment;
do not move or add cases merely to change layer counts. Cluster ownership is an
execution purpose, not a layer. `tests/smoke/` is outside the Next structure.

| CI division | Location / selector | Task |
| --- | --- | --- |
| Lint | repository checks | `lint` |
| Unit tests | colocated Next packages | `test:next-unit` |
| Library tests / non-cluster | `tests/library`; `library` and eligible `owned_cluster` or signing selectors | `test:next-library-non-cluster`, `test:next-library-non-cluster-lifecycle` |
| Library tests / in-cluster | `tests/library/cluster`; `library,cluster_integration` | `test:next-library-in-cluster` |
| CLI tests / non-cluster | `tests/cli`, `tests/cluster/lifecycle` | `test:next-cli-non-cluster`, `test:next-cli-non-cluster-lifecycle` |
| CLI tests / in-cluster | `tests/cluster`; `cluster_integration` | `test:next-cli-in-cluster` |

Keep the existing workflow build sharing, caches, dependencies, triggers,
permissions, and release conditions. Do not add sequencing between divisions.

Library tests and all their fixture or assertion helpers use only the public
UDS packages `pkg/bundle`, `pkg/bundle/spec`, and `pkg/iostreams`. They do not
invoke Cobra, internal UDS packages, or a UDS CLI binary, including indirectly
through their helpers. In-cluster library and CLI executions require
`KUBECONFIG` and `UDS_TEST_KUBECONFIG` to name the same explicit, Zarf-ready
kubeconfig provisioned externally with `hack/test-cluster.sh`. They clean up
test resources and preserve the cluster. Non-cluster tests may create an
isolated cluster with exact cleanup.

Credentialed library subsets use `library,signing_integration`:
`test:next-library-non-cluster-package-verification` verifies signed remote
packages, and `test:next-library-non-cluster-keyless` requires GitHub Actions
OIDC credentials.

Direct Cobra package-action callbacks require a standalone Zarf tool matching
the version in `go.mod`; use the mise-managed tool rather than another UDS
binary.

## Next errors

Error ownership is package-local. There is no repository-wide error package.

- Sentinels and named error types live in the owning package's `errors.go`.
- Named error methods, including `Error`, `Unwrap`, and `Is`, live in the owning `errors.go`.
- Operation-specific wrapping stays at the call site.
- Internal packages must not import `pkg/bundle/` just to reuse public errors.
- Public facades map internal failures to public sentinels or typed errors when callers need stable `errors.Is` or `errors.As` behavior.
- Use `%w`; do not log and return the same error.

## ADRs

Follow relevant ADRs before design or implementation work.

- For Legacy changes, read `docs/adr/legacy/`.
- For Next changes, read `docs/adr/`.
- For cross-mode work, read both sets, especially [`docs/adr/legacy/0010-legacy-next-coexistence.md`](docs/adr/legacy/0010-legacy-next-coexistence.md).
- Higher-numbered ADRs override conflicting lower-numbered ADRs in the same set.
- If a change deviates from an ADR, add a new ADR that supersedes the relevant section instead of rewriting history.

## Documentation and contributing updates

Keep user, contributor, and design docs synchronized with code changes.

- If you change user-visible behavior, CLI flags, command output, bundle semantics, examples, or workflows, update `docs/` or generated CLI docs.
- If you change setup, tasks, hooks, tests, release flow, or another contributor workflow, update `CONTRIBUTING.md` and/or `README.md`.
- If you change generated docs, run the corresponding docs generation or docs test task when practical.
- If you make a significant design decision, add a new ADR.

## Release workflows

- Ask before dispatching `.github/workflows/snapshot-release.yaml`. It writes to GHCR during validation and uses repository/package write permissions plus the `release-snapshot` environment to create remote tags and prereleases.
- Preserve unique snapshot tags: `vX.Y.Z-snapshot+YYYYMMDDHHMMSS-XXXXXXXX`. Never reuse, move, or force-update them. Keep this format excluded from `.github/workflows/release.yaml`.
- Scheduled cleanup removes snapshot prereleases beyond the newest three and preserves their tags. Keep `README.md`, `CONTRIBUTING.md`, and `AGENTS.md` synchronized with release workflow changes.

## Tooling and safety

- Lint: `uds run lint` or `hk check --all`
- Local preparation checks: `uds run test`
- See the testing skill for focused test tiers and commands.
- Ask before cluster operations, GHCR writes, destructive actions, or shared-state changes.
- Make the smallest root-cause fix, reproduce bugs close to user experience, and verify before declaring done.
