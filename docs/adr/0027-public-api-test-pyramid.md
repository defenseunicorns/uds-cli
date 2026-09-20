# 27. Public API test pyramid

Date: 2026-09-19

**Scope: Next CLI and its test suite only.** The Old (Legacy) CLI mode and test
suite are excluded from this ADR: no analysis, coverage calculation, migration,
or changes. `tests/smoke/` is outside this ADR and remains unchanged.

## Status

Proposed. Testing architecture for [CLI-302](https://linear.app/defense-unicorns/issue/CLI-302/cement-library-api-with-tests).

Implementation details: [plan.md](../../plan.md).

If accepted, this supersedes the test-layer classification in
[ADR-0002](0002-cli-architecture-patterns.md),
[ADR-0009](0009-bundle-deploy-from-artifact.md), and
[ADR-0014](0014-integration-test-tiers.md). Public package ownership from
[ADR-0020](0020-repository-package-structure.md) remains in force.

## Context

CLI-302 must cement the supported Go API, not the implementation behind it.
Executing a public function through Cobra, or testing its private adapter,
does not establish a direct consumer contract. Statement coverage alone also
cannot protect exported types, constants, result fields, or error identity.

### Status before the changes

The table below lists the existing Next test groups, their locations, case
counts, and scope.

| Test category | Location | Number of cases | What the tests exercise today |
| --- | --- | --- | --- |
| Unit tests | Next packages under `cmd/`, `internal/`, and `pkg/` | 496 (+1 example) | <ul><li>Parse and validate inputs.</li><li>Exercise private adapters.</li><li>Use files and local registries.</li><li>Check Cobra wiring.</li></ul> |
| Library tests | `tests/library/` | 25 | <ul><li>Call public bundle APIs.</li><li>Check errors, selection, and hooks.</li><li>Create, push, and inspect artifacts.</li><li>Stop before cluster deployment/removal.</li><li>Still use internal helpers.</li></ul> |
| Mixed integration | `tests/integration/` | 60 | <ul><li>Execute Cobra and the CLI binary.</li><li>Also call public APIs directly.</li><li>Check artifacts, registries, and signing.</li><li>Use some external fixtures.</li></ul> |
| Cluster E2E | `tests/cluster/` | 9 | <ul><li>Bootstrap a Zarf-initialized cluster.</li><li>Deploy and remove through the CLI.</li><li>Check signing and configuration.</li><li>Observe resources and operator events.</li></ul> |

The coverage table compares all Next code with its public API subset, per group
and cumulatively. Each cell reports covered statements and their percentage.

| Test category | Next alone | Next cumulative | Public alone | Public cumulative |
| --- | --- | --- | --- | --- |
| Unit tests | 4,447 (74.34%) | 4,447 (74.34%) | 744 (56.88%) | 744 (56.88%) |
| Library tests | 1,445 (24.16%) | 4,517 (75.51%) | 390 (29.82%) | 799 (61.09%) |
| Mixed integration | 3,140 (52.49%) | 4,826 (80.68%) | 633 (48.39%) | 938 (71.71%) |
| Cluster E2E | 3,094 (51.72%) | 4,953 (82.80%) | 465 (35.55%) | 983 (75.15%) |

The graph shows how coverage grows as each group is added, counting overlapping
statements only once.

```text
CUMULATIVE STATEMENT COVERAGE                Each # ~= 2 percentage points

Next production (5,982 statements)
Unit tests          |#####################################.............| 74.34%
+ Library tests     |######################################............| 75.51%
+ Mixed integration |########################################..........| 80.68%
+ Cluster E2E       |#########################################.........| 82.80%

Public API packages (1,308 statements)
Unit tests          |############################......................| 56.88%
+ Library tests     |###############################...................| 61.09%
+ Mixed integration |####################################..............| 71.71%
+ Cluster E2E       |######################################............| 75.15%

Bars span 0-100%; dots mean uncovered in the measured groups.
```

The main gap is direct library coverage: **29.82%** of public API statements.
Keyless signing remains unmeasured.

## Decision

### Three layers, classified by the boundary exercised

Build a testing pyramid: many unit tests, fewer library tests, and the fewest
CLI tests. The layer describes the interface under test. Cluster requirements
are a separate classification.

```text
                       /\
                      /  \
                     /CLI \          Command-line behavior
  Push tests down   /tests \
         |         /--------\
         |        / Library  \       Public API contracts
         |       /   tests    \      Library + CLI users benefit
         |      /--------------\
         v     /   Unit tests   \    Underlying logic
              /__________________\   Both consumer paths benefit
```

**Unit tests** protect the underlying logic: algorithms, parsing, validation,
and implementation-specific edge cases. They are the broad, fast base. Keep
white-box and black-box tests where useful. CLI-302 preserves this investment;
it does not reorganize existing unit tests.

**Library tests** exercise only the supported public APIs under `pkg/bundle`,
`pkg/bundle/spec`, and `pkg/iostreams`. They cement the contract that Go consumers
depend on. The CLI uses those APIs too, so these tests protect both library and
CLI users.

**CLI tests** execute Cobra directly. A minimal binary E2E check covers the
primary `cmd/uds` entrypoint and process-only behavior. They protect the
command-line contract: arguments, flags, aliases, config precedence, prompts,
output, error presentation, process exits, and Next-mode routing. Keep a small
number of complete workflows as end-to-end tests at the tip of the pyramid.
These workflows can also serve as smoke tests, exercising the CLI, public API,
and underlying implementation together. No separate smoke-test category is
needed; smoke testing describes their purpose within the CLI layer.

Command handlers return errors through Cobra; the executable owns reporting and
process exits. This supersedes the `util.CheckErr` convention in ADR 0002.

Both library and CLI tests have two subcategories:

| Subcategory | Cluster ownership | When to use |
| --- | --- | --- |
| In-cluster | ✗ | Test behavior against an existing Zarf-initialized cluster. Clean up test resources; preserve the cluster. |
| Non-cluster | ✓ (if needed) | Test behavior without an existing cluster, including creating, initializing, and deleting a test-owned cluster. |

**Push each behavior to the lowest layer that can prove it.**
Start with unit tests for the underlying logic. Use library tests when the
behavior is a public API contract. Use CLI tests when the behavior depends on
the command-line interface. Lower-layer coverage reaches more consumers:
a library test protects an API behavior for both Go callers and CLI users,
while a CLI-only test cannot establish that direct Go callers have the same
contract.

### Library tests must use only the public UDS API

Library tests must exercise UDS exclusively through public functions and methods
in `pkg/bundle`, `pkg/bundle/spec`, and `pkg/iostreams`. This rule includes fixture
and assertion helpers. No internal calls, Cobra invocation, or UDS CLI execution.

## Consequences

Public API changes become visible through compilation failures, result/error
assertions, and real operational effects. Internal refactors can proceed without
rewriting consumer tests. Most business-behavior coverage moves below Cobra,
while the existing unit investment and unique end-to-end coverage remain.

Cluster and credentialed library contracts still cost time and infrastructure.
They require explicit ownership and separate executions. During migration,
temporary overlap is expected: preservation of coverage and behavior takes
priority over immediately reducing test count or runtime.

### Test coverage after the changes

The complete Next matrix increased cumulative statement coverage. The changed
production denominator reflects the added source paths.

| Scope | Before | After | Change |
| --- | --- | --- | --- |
| All pyramid layers | 4,953 / 5,982 (82.80%) | 5,022 / 6,019 (83.44%) | +69 statements; +0.64 points |
| Public API | 983 / 1,308 (75.15%) | 1,044 / 1,315 (79.39%) | +61 statements; +4.24 points |

```text
CUMULATIVE STATEMENT COVERAGE                Each # ~= 2 percentage points

All pyramid layers before |#########################################.........| 82.80%
                    after  |##########################################........| 83.44%
Public API       before |######################################............| 75.15%
                 after  |########################################..........| 79.39%
```

The scenario inventory is 485 unit, 70 library, and 67 CLI cases. Credentialed
keyless signing remains a CI-only contract.
