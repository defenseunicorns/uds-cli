# CLI-302 implementation plan

**Scope: Next only.** Do not inspect, measure, migrate, or change the Old/Legacy
test suite. Preserve the existing unit-test investment.

Implement [CLI-302](https://linear.app/defense-unicorns/issue/CLI-302/cement-library-api-with-tests)
under [ADR-0027](docs/adr/0027-public-api-test-pyramid.md). The ADR records the
testing architecture; this file records implementation work and acceptance
evidence. The ADR is still proposed. This plan does not authorize implementation,
commits, workflow dispatches, or publication by itself.

## Outcome and constraints

- Cement the consumer contract of `pkg/bundle`, `pkg/bundle/spec`, and
  `pkg/iostreams` through direct public API tests.
- Add successful operations, meaningful result assertions, public error
  identity, constants, hooks, extension points, and resource lifecycle checks.
- Move business-behavior assertions below Cobra after replacements pass.
- Preserve covered production statements and unique CLI/security assertions.
  Increase cumulative Next coverage, cumulative public API coverage, direct
  library coverage, and the number of verified API contracts.
- Finish with more unit scenarios than library scenarios, and more library
  scenarios than CLI scenarios, classified by the boundary exercised.
- Do not add production exports, restore removed injection seams, rewrite units,
  introduce a test framework or nested Go module, or change API behavior merely
  to simplify tests.
- Internal packages may be read to understand behavior. Library tests and their
  helpers must not import or call them, invoke Cobra, or execute the UDS binary.
  Internal execution behind a public API call is expected and allowed.

## Terra implementation and Sol review

Use the six phases below in order. Their checkboxes are implementation status;
none is completed by writing this plan.

**Terra:** implement one bounded phase or contract group at a time. Read its
listed production entry points, existing tests, and applicable repository skills
first. Check the worktree before edits. Reuse neutral helpers and existing
dependencies. Keep assertions tied to consumer behavior, not internal adapters.

**Sol:** review the resulting diff and evidence before the phase closes. Check
the public-only boundary, fixture validity, meaningful assertions, preserved
behavior, cleanup ownership, and coverage accounting. Identify a source-backed
defect or an unmet acceptance condition for each requested correction.

For every handoff, record:

1. Phase and contract rows addressed; exact files changed.
2. Public symbols and behaviors asserted, with test names.
3. Tests executed, outcomes, skips, runtime, and coverage artifact locations.
4. Any remaining dependency, failure, or contract ambiguity.
5. Sol findings and their resolution.

Do not treat “compiles,” “returns no error,” or a helper's incidental API call
as proof of a result contract. If observed behavior conflicts with a proposed
assertion, report the exact public call and result. Resolve the contract before
cementing an accidental implementation detail or changing production behavior.

## Baseline to preserve

Research revision: `acace437e45ed8727d4ebfd65359b9303584afea`.
Measured 2026-09-19 with Go 1.26.6 on darwin/arm64.

| Existing execution | Cases / outcome | Wall time | Next statements alone | Public statements alone |
| --- | --- | --- | --- | --- |
| Colocated units | 496 tests + 1 example passed | 44.42 s | 4,447 / 5,982 | 744 / 1,308 |
| Library | 25 passed | 12.75 s | 1,445 / 5,982 | 390 / 1,308 |
| Mixed integration | 59 passed; 1 keyless case skipped | 1,125.46 s | 3,140 / 5,982 | 633 / 1,308 |
| Cluster E2E | 9 passed | 381.75 s | 3,094 / 5,982 | 465 / 1,308 |

Coverage floors for comparable production source:

| Combination | Next covered statements | Public covered statements |
| --- | --- | --- |
| Units + default library + mixed integration | 4,826 / 5,982 (80.68%) | 938 / 1,308 (71.71%) |
| Above + Cluster E2E | 4,953 / 5,982 (82.80%) | 983 / 1,308 (75.15%) |

The first combination has its own preservation gate. Compare all required PR
executions and the complete infrastructure matrix too.
Do not infer contract quality from the percentages alone.

Research evidence currently lives in `/tmp/uds-cli-302-coverage/`:

- `next-unit.txt`, `library.txt`, and each of `cli` and `cluster` profiles.
- Corresponding `*-binary.txt` profiles and raw `*-binary/` coverage directories.
- `next-summary.json`, `next-packages.txt`, JSON test logs, and `*.run.json`.

This directory is temporary, not a repository dependency. Preserve evidence in
the implementation's review artifacts before it disappears, or rerun the
baseline on the same revision. Do not commit credentials, kubeconfigs, tokens,
or generated signing keys. Go version/platform changes require a matching
before/after measurement; the Mac baseline is not a Linux CI threshold.

Keyless signing remains unmeasured without GitHub Actions OIDC identity.

Counts exclude helpers, `TestMain`, and nested table rows; suite methods replace
their wrapper entrypoints. Timings include compilation and uncontrolled network
and cache effects. The instrumented binary build took another 45.23 seconds.

## Existing contract gaps

The ticket's inventory needs these corrections:

- [remove_artifact_test.go](tests/library/remove_artifact_test.go) already
  successfully calls `Inspect` for local and OCI artifacts, but only checks the
  digest. It imports `internal/bundle` and `tests/testutil`. The latter depends
  on `internal/testutil`, whose fixture factory uses Cobra's config resolver.
- Successful direct `Pull` calls exist in the CLI runner, including signed OCI
  pull. Successful key-based Sign/Verify through Cobra do not require a cluster.
- [zarf_test.go](pkg/bundle/zarf_test.go) already tests loader adapters,
  digest mutation, staging-root behavior, adjacent defaults, and skipped-result
  counting. These implementation tests do not replace external API contracts.
- `GlobalOptions` is retained for compatibility. Current public configuration
  adapters do not forward it; `ConfigOptions` controls operations. There is no
  public configuration-parser API to test. Do not invent a Global round-trip
  contract or call the internal parser from library tests.
- The `ClusterDeployFn` and `PackageDeployFn` seams described in ADR-0014 are no
  longer public. Do not restore them merely to simulate successful deployment.

## Infrastructure and execution

Keep existing locations and task entrypoints while migrating their contents.
Add narrowly scoped library executions where infrastructure requires them.

These are execution groups, not extra pyramid layers. Library and CLI tests each
have **in-cluster** and **non-cluster** subcategories:

- In-cluster requires an existing, running, Zarf-initialized cluster.
- Non-cluster requires no existing cluster. It may create and initialize one.
- A cluster-creating library test still uses public UDS APIs for UDS operations.
  Creating infrastructure never makes a Cobra test a library test.

| Execution | Location / tags | Starting requirements |
| --- | --- | --- |
| Default library | Existing `tests/library/`; `library` | Non-cluster; local files, generated keys, localhost registry. No UDS binary, external identity, or public-registry downloads. |
| Cluster library | Proposed `tests/library/cluster/`; `library && cluster_integration` | Supplied-cluster contracts enter the in-cluster library run; creation cases enter the non-cluster library run. |
| Credentialed library | Proposed `tests/library/signing/`; `library && signing_integration` | Conditional subset of the non-cluster library run; real signing identity and verification services. |
| CLI integration | Existing `tests/integration/`; `integration` | Short Cobra and primary-binary checks. Reduce large business fixtures. |
| CLI E2E | Existing `tests/cluster/` | Split supplied-cluster and cluster-creation scenarios by the ADR definitions. |

The two new library subdirectories are proposed. Route their tests into the
six runs below; directories and build tags do not require separate jobs.

Default unit, library, and short CLI tests run on ordinary PRs. Keep cluster
library and CLI assertions in their respective test runs without adding
sequencing between them or changing the existing build/workflow logic.
Run real keyless contracts as a credentialed subset of Library Tests / non-cluster;
missing credentials must fail an execution requiring that subset. Keep Core as an
optional subset of CLI Tests / non-cluster. Next test-suite changes must
update `CONTRIBUTING.md`, the testing guide, and affected `AGENTS.md` guidance.

Supplied clusters require an explicit test kubeconfig and test-owned namespaces;
cleanup must never delete the supplied cluster. Create-cluster mode must use a
unique name, an isolated kubeconfig, and cleanup only resources created by that
run. It must not silently fall back to the user's current context. Provisioning
tools such as k3d may establish infrastructure; UDS bundle operations, including
fixture bootstrap, must use the public library. Prefer fixtures without Zarf
actions that call an executable. Any unavoidable system-Zarf callback dependency
must be documented and tested; do not disguise an UDS binary test as a library
test. The existing cluster `TestMain` cannot be reused unchanged.

CI provisions supplied clusters outside the suites with `hack/test-cluster.sh`.
The helper only sets up or cleans up infrastructure; workflow steps run the tests.
It uses mise-installed Zarf and its matching init package (v0.86.0, aligned with
go.mod). It performs no UDS bundle operations. The owned library lifecycle
separately tests public-API bootstrap. Supplied-cluster tests receive an explicit isolated
kubeconfig and never create or delete the cluster. CI saves diagnostics before
an unconditional cleanup step; existing clusters are never adopted or deleted.

Do not place external-fixture scenarios in the default local library subset merely
because they need no Kubernetes. Keep such scenarios in explicitly selected
subsets of the non-cluster run. Replace external fixtures with faithful local
ones where possible. If a real external fixture is essential, give that scenario
an explicit subset and preserve its required CI coverage before migration.

Proposed contract file names below are suggestions, not existing files or a
requirement to create one file per row. Group closely related contracts.

## Next test-suite divisions

Phase 5 divides the Next test runs into the six groups below. This is a test-suite
change, not a build or workflow redesign. Reuse all existing CI optimizations,
including shared builds, binary artifacts, tool/dependency/compilation caches,
setup actions, runner configuration, cancellation, and safe test parallelism.
Preserve workflow dependencies, triggers, permissions, and release conditions.
Do not add sequencing between test groups. Legacy workflows remain untouched.

### What executes today

Source: [test workflow](.github/workflows/test.yaml),
[test tasks](tasks/tests.yaml), [PR caller](.github/workflows/test-pr.yaml),
[release caller](.github/workflows/test-release.yaml),
[setup action](.github/actions/setup-uds-e2e/action.yaml), and
[tool/cache setup](.github/actions/mise/action.yaml).

- Library tests start independently and already need no built UDS binary.
- CLI integration and cluster jobs wait for the binary build, then run in
  parallel.
- The setup action always downloads the UDS binary. `create-cluster: false`
  disables provisioning, not that dependency. Do not use it for library jobs.
- The colocated unit task spans both modes. Add an explicit Next-only execution
  for Next evidence; preserve the existing shared task/job unchanged.
- Next cluster tests bootstrap Zarf and a cluster in `TestMain`, then share that
  cluster with parallel cases (`-parallel=4`). They are non-cluster by the ADR's
  starting-condition definition.
- CLI integration includes real keyless signing and the large Core Create case.
  Move those responsibilities before treating it as the short CLI execution.
- Go/tool caches and PR cancellation already exist. The library task lacks
  `-count=1`; cached test results must not stand in for fresh contract execution.

Three successful PR runs sampled on 2026-09-20 show the following job durations,
including setup and teardown but excluding time before the job starts:

| Run | Binary build | Library | CLI integration | CLI cluster |
| --- | --- | --- | --- | --- |
| [35497450532](https://github.com/defenseunicorns/uds-cli/actions/runs/35497450532) | 3m 57s | 55s (test result cached) | 4m 21s | 5m 21s |
| [35388507988](https://github.com/defenseunicorns/uds-cli/actions/runs/35388507988) | 4m 40s | 4m 11s | 3m 39s | 5m 17s |
| [35388335331](https://github.com/defenseunicorns/uds-cli/actions/runs/35388335331) | 44s | 3m 41s | 6m 11s | 8m 24s |

These runs use different revisions and uncontrolled caches; they are context,
not a before/after benchmark. The latest library log explicitly reports
`(cached)`. Its Core Create case took 66.63s, versus 706.43s in the local research
run. Do not promise the local saving in CI. In run 35388335331, the build did not
start until 23m 41s after run creation; inspect scheduling and runner capacity
before assuming more jobs always improve feedback time. No cause for that delay
has been established.

### Six test runs

Divide test selection and responsibility as follows. These groups do not prescribe
new workflow files, build jobs, runner topology, or artifact-transfer logic.

| Run | Responsibility |
| --- | --- |
| Lint | Existing lint checks and the Next public-library boundary check, using current tools and rules. |
| Unit Tests | Next colocated tests, preserving the existing investment. Classify any Cobra cases as CLI in the scenario inventory without moving their files. |
| Library Tests / non-cluster | Public API contracts with no supplied cluster: local artifacts/registries, focused cluster-creation lifecycle cases, and credentialed signing in its eligible execution. |
| Library Tests / in-cluster | Public API deployment/removal contracts against a supplied Zarf-initialized cluster. Tests clean up their resources and preserve the cluster. |
| CLI Tests / non-cluster | Cobra/primary-binary contracts with no supplied cluster, including bootstrap workflows. Core E2E retains its existing conditional execution. |
| CLI Tests / in-cluster | Cobra/primary-binary contracts against a supplied Zarf-initialized cluster. Tests clean up their resources and preserve the cluster. |

Non-cluster tests may create clusters; retain the existing cluster-capable
execution environment where needed. Keyless and Core remain subsets, not extra
test categories. Preserve their existing credential and release requirements.

Split the existing Next CLI cluster assertions by responsibility. Bootstrap and
cluster-ownership assertions belong in the non-cluster run. Supplied-cluster
Deploy/Remove assertions belong in the in-cluster run. Adapt test selection and
test-owned fixtures accordingly; do not redesign CI provisioning to achieve the
split. Preserve assertions and safe case parallelism without duplicating suites.

### Implementation and performance checks

- Change Next test grouping, selectors, and fixture ownership only. Reuse existing
  tasks and execution infrastructure. Do not introduce a new workflow, per-job
  CLI builds, a shared-build replacement, or a new aggregation job.
- Only minimal process-entrypoint checks use the existing built binary and
  artifact distribution; other CLI tests execute Cobra directly.
  Library tests and their fixtures must still invoke only the public UDS API;
  they must not depend on executing that binary.
- Reuse existing caches and optimizations. Use fresh test runs for coverage and
  before/after timing evidence; do not mistake cached test results for execution.
  This measurement requirement does not authorize redesigning cache behavior.
- Preserve cluster reuse within a suite where safe. Separate creation ownership
  from supplied-cluster assertions in the test harness. Keep explicit kubeconfigs,
  resource isolation, cleanup, and the library's public-only fixture boundary.
- Keep real keyless coverage in its existing eligible credentialed execution,
  classified as Library Tests / non-cluster after migration. Keep Core in its
  existing enabled execution, classified as CLI Tests / non-cluster. Neither
  migration authorizes changes to permissions, triggers, or release gating.
- Reuse existing result/diagnostic handling. Collect per-run coverage and timing
  for Phase 6, and merge the profiles as review evidence. Preserve the default
  and full-matrix gates without introducing workflow dependencies or rerunning
  all tests serially to obtain their union.
- Attribute performance changes to the test-suite split and migrated assertions.
  Compare fresh runs on matched source and runner types; report suite time and
  total Next feedback time. Keep build/setup costs visible and unchanged so that
  the comparison does not credit an unrelated workflow optimization.

## CLI-302 contract buckets

The matrix includes the ticket gaps and the existing Create/Push and shared
public contracts needed to make their migration reviewable. Keep the existing
unit edge-case coverage; add focused external consumer assertions.

| Ticket gap | Owning execution | Required contract |
| --- | --- | --- |
| Create and `CreateResult` | Default library | Direct Create success; assert `BundleName`, output path next to the source definition, readable artifact, defaults/values, packaged package identities, optional-component inclusion/exclusion, architecture, and signing policy. Migrated Create scenarios must assert the public result rather than discard it in a helper. |
| Push and `PushResult` | Default library | Direct Push to a localhost registry; assert the returned OCI reference, then Pull/Inspect that reference to prove artifact identity and signature preservation. |
| Public domain model (`pkg/bundle/spec`) | Default library | Construct public domain types used by operations; validate valid/invalid bundles directly and assert public sentinels, typed errors, and their fields. Keep private parsing and exhaustive algorithm cases in units. |
| Public streams (`pkg/iostreams`) | Default library | Exercise public constructors/accessors and caller-supplied logging through a bundle operation; assert input/output/diagnostic routing, zero-value safe output, and unconfigured leveled logging as a no-op. Assert behavior through public methods and buffers, never private locks or logger binding. |
| Inspect and summary types | Default library | Local and localhost-OCI success; name, description, version, digest, provenance, parsed `Bundle`, package source/namespace/dependencies/values, and dependency order. Assert bundle `verified`, `not_checked`, `skipped`, and `BundleSignatureStatusUnverified == BundleSignatureStatusNotChecked`; package signing and verification statuses, including unknown metadata. Package metadata must not imply bundle authentication. |
| Reconfigure and `ReconfigureResult` | Default library | Local and localhost-OCI success; correct populated result field and empty alternative; readable output with new defaults/name/provenance, unchanged input, and source/output signature policy. Test a real re-sign/verify path. |
| Pull and `PullResult` | Default library | Push/Create fixture through public APIs; assert returned reference, output path under the requested directory, readable artifact identity, and signed verification. Preserve unsigned-policy failure coverage. |
| Sign / Verify | Default library | Real generated-key success for local and localhost-OCI sources; verify with the expected key; reject a different key and altered content using public errors. These functions return errors, not result structs. |
| `PrepareDeploySource` / `Close` | Default library | Directory/HCL and artifact success; populated `BundlePath`, optional `DefaultsPath`, artifact `Bundle` and `Loader`, usable values files, and source-owner cleanup. Repeated `Close`, nil/zero-value close, preservation of caller files, and failure cleanup. For OCI, call public Pull first; preparation itself is not an OCI pull or signature-verification API. |
| `ZarfPackageLayout.SetDeployedDigest` | Default library, then cluster library | Setter/getter round trip on a non-nil layout; nil `SetDeployedDigest` is a no-op. Do not assume nil `Digest` or `DirPath` is safe; hook/loader digest survives a real deployment and is observable in deployed package identity. |
| `PackageStagingRootProvider` | Default library | Public custom loader/provider through Deploy; observe supplied destination and public layout path. Cover preferred root, empty-root fallback, and unavailable-root fallback. A deliberate pre-deploy stop is sufficient for these staging contracts, not deployment success. |
| Custom `ZarfPackageLayoutLoader` | Default and cluster library | Stage a valid package through the documented public interface; assert load options, partial status, hook-visible definition/path/digest, and error propagation. Add successful deployment with the custom loader; no private adapter calls. |
| `Variables` / `ConfigOptions` | Default library; cluster library for deployed values | Public configurations with scalars, nested `Variables`/maps, and lists; assert materialized/deployed values and hook-visible `Variables`/`ConfigOptions`. |
| Compatibility `GlobalOptions` | Default library | Compare observable operation behavior with nil and non-nil `Global`, holding operational `ConfigOptions` fixed. Do not assert that `Global` survives conversion or appears in hooks. |
| Deploy and `DeployResult` | Cluster library | Successful source and prepared-artifact deployment, including Pull → Prepare → Deploy for OCI; assert `BundleName`, selected package results, actual resources, and successful pre/post hook execution. Exercise supplied-cluster and create-cluster modes. |
| Remove and `RemoveResult` | Cluster library | Remove genuinely deployed source/artifact/OCI fixtures; assert public bundle/package results and resource removal while preserving shared Zarf infrastructure. |
| `RemovePackageStatusSkipped` | Cluster library | Remove an already-absent package on an initialized cluster; assert successful skipped status through the public result, including a mixed removed/skipped case. |
| `SigningModeKeyless` | Credentialed library | Real public Sign → Verify success with constrained certificate identity and issuer; wrong-identity rejection. A validation-only test or local skip does not close this gap. |

Unsigned fixtures are deliberately unauthenticated test inputs. Keep them
separate from real signature-success assertions. Expected public behavior is
grounded in [inspect.go](pkg/bundle/inspect.go),
[reconfigure.go](pkg/bundle/reconfigure.go),
[pull.go](pkg/bundle/pull.go), [sign.go](pkg/bundle/sign.go),
[verify.go](pkg/bundle/verify.go), [deploy.go](pkg/bundle/deploy.go),
[remove.go](pkg/bundle/remove.go), [zarf.go](pkg/bundle/zarf.go), and
[configuration.go](pkg/bundle/configuration.go). See also [create.go](pkg/bundle/create.go),
[push.go](pkg/bundle/push.go),
[spec/validation.go](pkg/bundle/spec/validation.go), and
[iostreams.go](pkg/iostreams/iostreams.go). Private code was inspected for
this research; it is not an API the proposed tests may call.

## Ordered implementation schedule

Complete each phase and its Sol review before deleting or moving coverage in a
later phase. Keep unrelated files out of each handoff.

### Phase 1 — Baseline, fixtures, and the public boundary

- [x] Preserve or refresh the baseline using the measurement requirements below.
- [x] Classify existing Next scenarios by their exercised boundary, independently
  of runner, directory, or build tag. Record the initial pyramid counts.
- [x] Inventory exported symbols in the three public packages and map existing
  consumer assertions. Include functions, methods, fields, constants, aliases,
  interfaces, sentinels, and typed errors. Record missing assertions explicitly.
- [x] Remove internal imports and indirect Cobra coupling from existing library
  tests without dropping their current error/hook assertions.
- [x] Add the library import-boundary check and wire it into a Next check.
- [x] Establish small, valid, reusable local fixtures.

Pyramid inventory rules:

- Record each scenario's test name, unit/library/CLI boundary, current execution,
  intended owner, and migration decision. For library/CLI scenarios, also record
  in-cluster/non-cluster status and whether execution creates a cluster.
- Count top-level tests or suite methods consistently before and after; exclude
  helpers, `TestMain`, wrapper duplication, and nested table rows. Report examples
  separately. Running a scenario in two cluster modes does not create two cases.
- Inspect bodies and helpers. Colocated Cobra executions belong to the CLI layer;
  isolated private function tests remain units. Keep their existing files and
  execution intact. Runner names alone are not the architectural inventory.
- Mark implementation-coupled API tests as pending boundary repair; they do not
  qualify as completed public-only library contracts until their helpers comply.
- Preserve the five-group coverage baseline as execution evidence. Do not infer
  three-layer counts from it or claim per-layer coverage from mixed profiles.

Read first:

- [Existing library tests](tests/library/), especially
  [artifact removal fixtures](tests/library/remove_artifact_test.go) and
  [public errors](tests/library/public_errors_test.go).
- [Current shared helpers](tests/testutil/) and
  [their internal implementation](internal/testutil/bundle.go), for research only.
- [Public validation](pkg/bundle/validation.go),
  [configuration](pkg/bundle/configuration.go), and [errors](pkg/bundle/errors.go).
- [Architecture check](hack/check-architecture.sh) and [test tasks](tasks/tests.yaml).

Fixture rules:

1. Use `package bundle_test` or another external consumer package. Construct
   public configuration directly. Set an explicit architecture, temporary root,
   and valid options; use `PlainHTTP` only for the local test registry.
2. Build tiny valid Zarf fixture contents using existing dependencies or stable
   fixture files, then call public `bundle.Create` for UDS artifacts. Include
   files, metadata, and checksums needed for real successful loading. Do not
   assume the minimal validation-only fixture can actually deploy.
3. Keep fixtures deterministic and per-test isolated. Add a localhost registry
   and generated key pair only where needed. Register cleanup immediately.
4. Do not import `tests/testutil` wholesale: it reaches `internal/testutil` and
   Cobra configuration. Extract only neutral functions actually reused, or keep
   tiny helpers in the library test package. Do not introduce a helper framework.
5. Use public Inspect/Prepare/Verify results and externally observable files or
   Kubernetes resources for assertions. Archive readers may use existing neutral
   dependencies; they must not call private UDS extractors or inspectors.
6. Keep unsigned structural fixtures distinct from authentic signature fixtures.
   Metadata saying “signed” is not evidence of a valid signature.
7. Use the standard library and existing dependencies for infrastructure.
   Third-party types exposed by the public API remain usable. Do not assert
   private adapter state or staging-directory naming conventions.

Boundary check requirements:

- Inspect test files and their repository-owned helper imports for every library
  tag combination, including cluster and signing. Include external test files.
- Reject UDS `internal/...`, Legacy packages, and direct Cobra imports in that
  test/helper graph. Reject helper paths that lead to those forbidden imports.
- Stop traversal at the supported production packages `pkg/bundle` (including
  `spec`) and `pkg/iostreams`. Their private implementation dependencies are legal.
- Existing third-party libraries are allowed; do not inspect their implementation
  imports as though they were test-owned UDS code.
- An import check cannot prove subprocess behavior. Sol must also check for UDS
  execution through `exec`, wrappers, environment-configured paths, or fixtures.
  Provisioning tools such as k3d remain allowed for infrastructure.
- Keep this guard Next-specific. Do not repurpose the current Legacy architecture
  check or change its scope. Add a small negative/positive verification for the
  new guard: reject a helper's internal import; allow a public facade's internals.

**Phase exit:** existing library contracts pass; the boundary check passes for
all implemented variants; fixtures require no CLI parser or UDS executable.

Phase 1 verified: 25 library cases pass; the boundary guard and its regression
check pass. Sol found no issues. The boundary inventory records 485 unit, 32
API-oriented, and 74 CLI scenarios, plus one example. The seven API scenarios
remaining in the mixed suite still require public-only migration. Original
profiles, the scenario inventory, and exported API declarations are retained
under `/tmp/uds-cli-302-coverage/` and `/tmp/uds-cli-302-implementation/`.
Existing consumer assertions cover input/error contracts, selection and aborting
hooks, and artifact digests; successful operation results, lifecycle ownership,
configuration conversion, spec errors, and streams remain Phase 2/3 work.

### Phase 2 — Default library contracts

- [x] Implement every row below through public calls.
- [x] Preserve existing CLI scenarios until phase 5.
- [x] Update the symbol-to-test mapping with actual assertions.

Suggested files: `tests/library/artifact_contract_test.go`,
`signing_contract_test.go`, `source_lifecycle_test.go`,
`extension_contract_test.go`, and `configuration_contract_test.go`.

| Contract | Required setup and assertions |
| --- | --- |
| Create / `CreateResult` | Create a local bundle. Assert `BundleName`, `OutputPath` next to the source, readable artifact, package identities, included defaults/values, architecture, optional-component inclusion/exclusion, and signing policy. `CreateOptions` has no package-selection field. |
| Push / `PushResult` | Push to localhost. Assert `OCIReference`; Pull and Inspect that reference to confirm identity and signature preservation. Do not discard the public result in a helper. |
| Pull / `PullResult` | Pull the pushed fixture into a requested directory. Assert `OCIReference`, `OutputPath`, file existence, artifact identity, successful signed verification, and preserved unsigned-policy failures. |
| Inspect / `InspectResult` | Inspect local and localhost-OCI fixtures. Assert `Name`, `Description`, `Version`, `ArtifactDigest`, `ReconfiguredFrom`, parsed `Bundle`, and meaningful `Packages` fields. Check dependency-before-dependent order; do not impose sibling order without a contract. |
| Inspect summaries | Assert `PackageSummary` name, source, namespace, dependencies, values-file references, and signature metadata. Cover signed/unsigned/unknown metadata and verified/skipped/unknown package verification posture using valid fixtures. Package metadata does not authenticate the bundle. |
| Signature-status constants | Assert bundle `verified`, `not_checked`, `skipped`, and the unverified/not-checked alias; assert all package signing and verification constants from `inspect.go`. Declarations are not measured by statement coverage. |
| Reconfigure / `ReconfigureResult` | Exercise local and localhost-OCI sources. Local output populates `OutputPath`; OCI output populates `OCIReference`; assert the unused field is empty. Inspect the new name/provenance, verify new defaults, preserve input identity, and test source-verification and output-signing policies. Re-sign and verify a real result. |
| Sign / Verify | Generate a real key pair. Sign then Verify local and localhost-OCI artifacts; reject a different key and altered signed content. Assert public error identity. These APIs return errors, not result objects. |
| Signing/verification options | Cover the three `SigningMode` values and invalid combinations through public validation/operations. Key signing belongs here; actual `SigningModeKeyless` success belongs in phase 4. |
| Prepare source / Close | Prepare directory, HCL-file, and artifact inputs. Assert absolute `BundlePath`, optional `DefaultsPath`, artifact `Bundle` and `Loader`, and usable values files. Directory/HCL preparation need not populate artifact-only fields. Close must remove owned temporary resources, preserve caller files, tolerate repetition, and accept nil/zero-value sources. Exercise failure cleanup. |
| Layout digest | On a non-nil `ZarfPackageLayout`, call `SetDeployedDigest` and assert `Digest`. Nil `SetDeployedDigest` is safe; nil `Digest` and `DirPath` are not promised safe. Real deployed identity is checked in phase 3. |
| Custom loader and staging | Implement `ZarfPackageLayoutLoader` and optional `PackageStagingRootProvider`. Stage a valid package into the supplied destination; return the public layout/result. Observe load options, partial status, definition/path/digest through hooks, and error propagation. Cover preferred staging root, empty-root fallback, and unavailable-root fallback. |
| Variables / operational configuration | Pass scalars, nested `Variables`, maps, and lists via `UDSBundleConfig`. Observe materialized values and hook-visible configuration. Verify relevant `ConfigOptions` through effects, not calls to private converters. Deployed values are checked in phase 3. |
| Compatibility `GlobalOptions` | Hold `ConfigOptions` fixed and compare behavior with nil and non-nil `Global`. Current adapters omit `Global`; do not invent a round-trip or hook-visibility contract. |
| Public domain model | Construct `spec.UDSBundle` and related public types. Call `Validate` for valid/invalid cases; assert public sentinels and typed errors with their fields, including joined errors. Keep exhaustive private parser cases in units. |
| Public streams | Exercise `New`, `NewTestIOStreams`, accessors, and caller-supplied `slog` logging. Assert input/output/diagnostic routing and a real bundle operation using supplied streams. Zero-value output is safe; unconfigured leveled logging is a no-op. Do not inspect locks or private logger binding. |

Implementation traps:

- `VerificationPolicy.PublicKey` contains PEM text. Read the generated public
  key file; do not pass the path accepted by the CLI flag. `SigningOptions.Key`
  supplies the private-key reference used by the signing API.
- For OCI deployment preparation, call Pull first and Verify as appropriate,
  then `PrepareDeploySource`. Preparation does not pull OCI or authenticate it.
- A custom loader populates `dstDir`; it cannot set private layout path fields.
  Observe the resulting path through public hooks/accessors.
- A deliberate pre-deploy abort is valid for staging, loading, and hook-error
  contracts. It is not successful deployment coverage.
- Use public `errors.Is`/`errors.As` contracts. Do not match an internal sentinel
  or freeze an incidental full error string/log line.
- Test meaningful round trips rather than reproducing private implementation
  steps. Mutating an archive for tamper tests is fixture manipulation, not a
  substitute for an actual Sign/Verify success path.

Read the public implementations before each group:
[Create](pkg/bundle/create.go), [Push](pkg/bundle/push.go),
[Pull](pkg/bundle/pull.go), [Inspect](pkg/bundle/inspect.go),
[Reconfigure](pkg/bundle/reconfigure.go), [Sign](pkg/bundle/sign.go),
[Verify](pkg/bundle/verify.go), [source/hooks](pkg/bundle/deploy.go),
[layouts](pkg/bundle/zarf.go), [domain validation](pkg/bundle/spec/validation.go),
[domain errors](pkg/bundle/spec/errors.go), and [streams](pkg/iostreams/iostreams.go).

**Phase exit:** all default contracts pass without a cluster, UDS binary, or
external service; result/constant/error assertions are mapped; direct public
coverage has increased. Any fixture requiring external services is explicit.

#### Phase 2 evidence and contract map

Fresh default-library run: 61 passed, no failures or skips; 25.88 s including
instrumentation/build. Public coverage: 920/1,315 (69.96%), up from 390/1,308
(29.82%). Next coverage: 2,561/5,989 (42.76%). Profiles and JSON outcomes:
`/tmp/uds-cli-302-implementation/coverage/library.*`. Sol approved the contracts.
Final lint: zero issues. Library rerun after fixture lint fixes: 5.050 s, passed.
The boundary guard and its tests passed. The full default-library race run
passed in 26.822 s.

The denominator grew by seven statements: `PrepareDeploySource` now honors its
documented absolute-path contract for relative source and temporary paths.
The regression fails against the original production source. Fresh Next units
passed (496 tests plus one example); all previously covered unit blocks remain
covered under the recorded source-block mapping.

| Public contract | Assertions in `tests/library/` |
| --- | --- |
| Create/Inspect results; package summaries; optional components; architectures | `TestCreateAndInspectPublicContract`, `TestCreateOptionalComponentCanBeExcluded`, `TestCreatePublicContractSupportsMultipleArchitectures` |
| Package signature states and independent bundle state | `TestCreateInspectsSignedVerifiedPackage`, `TestCreateRejectsWrongPackageVerificationKey`, `TestInspectUnknownPackageMetadata`, `TestInspectStatusConstants` |
| Push/Pull results, artifact identity and signature preservation | `TestPushPullAndInspectPublicContract`, `TestPushPreservesBundleSignature` |
| Local/OCI Sign/Verify, wrong key and tampering | `TestSignVerifyPublicContract`, `TestSignVerifyOCIContract` |
| Local/OCI Reconfigure results, defaults, provenance, source preservation and verification | `TestReconfigureLocalPublicContract`, `TestReconfigureOCIPublicContract`, `TestReconfigureVerifiesSourcePolicy` |
| Signing/verification modes, options and validation | `TestPublicSigningModeValues`, `TestPublicSigningOptionsValidate`, `TestPublicSignOptionsValidate`, `TestPublicVerificationPolicyValidate`, `TestPublicVerifyOptionsValidate` |
| Source fields, caller ownership, cleanup, repeated/nil Close and relative paths | `source_lifecycle_test.go`: six lifecycle contracts |
| Layout digest, loader options/results/errors, staging roots and cleanup | `TestLayoutDeployedDigestContract`, `TestCustomLoaderStagingAndCleanup`, `TestCustomLoaderErrorPreservesIdentityAndCleansStaging` |
| Variables, ConfigOptions, defaults precedence, Global compatibility, validation and value templates | `configuration_contract_test.go`: five configuration contracts |
| DependencyViolationError public fields | `TestDeploySubset_LeafWithoutDepBlocked` in `deploy_subset_test.go` |
| spec model validation, three sentinels, nine typed errors and fields | `TestPublicBundleValidationContracts` |
| IOStreams constructors/accessors/logging, supplied logger and zero values | `TestPublicIOStreamsContracts`, `TestPublicIOStreamsCallerLoggerReachesBundleOperation` |

Successful deployment, persisted layout digest, removal statuses and real keyless
identity remain Phase 3/4 gates. Hook-abort tests above do not satisfy them.

### Phase 3 — Real cluster library contracts

- [x] Implement isolated in-cluster and non-cluster creation modes.
- [x] Add successful Deploy/Remove contracts and extension-point success paths.
- [ ] Include these contracts in Library Tests / in-cluster, with creation
  lifecycle cases in Library Tests / non-cluster, using existing CI infrastructure.

Start with [public Deploy](pkg/bundle/deploy.go), [Remove](pkg/bundle/remove.go),
and [layout handling](pkg/bundle/zarf.go). Read [cluster setup](tests/cluster/main_test.go)
and [helpers](tests/testutil/cluster.go) for existing infrastructure behavior;
do not reuse their CLI bootstrap in library tests.

Infrastructure requirements:

1. In-cluster mode requires an explicit kubeconfig and verifies Zarf readiness.
   Fail clearly when the supplied cluster is missing/uninitialized; do not
   silently create one or use the developer's current context.
2. Non-cluster mode creates a uniquely named cluster with an isolated kubeconfig
   and available ports. Initialize Zarf using an existing supported public Go
   operation/dependency, never an UDS CLI wrapper. Establish the bootstrap path
   before building the full fixture set; do not add a production export for it.
3. Use valid deployable packages with small observable resources. Prefer existing
   neutral fixture contents under `tests/test_data/` after checking their
   dependencies. Do not bootstrap full Core for ordinary API contracts.
4. Give every test isolated names/resources. Collect diagnostics on failure;
   remove test-owned resources even after assertion failure. Delete only a
   cluster created by this run. A supplied cluster and its Zarf installation stay.
5. Run both modes in validation. Avoid duplicating every expensive scenario:
   run bulk contracts in Library Tests / in-cluster and focused creation/cleanup
   contracts in Library Tests / non-cluster. Reuse neutral helpers, not entire
   duplicate executions.
6. Do not use parallel tests for process-global kubeconfig/environment mutation.
   Synchronize callback observations when deployment hooks run concurrently.

| Contract | Required assertions |
| --- | --- |
| Deploy success | Deploy a source definition and a prepared artifact. Include an OCI Pull → Prepare → Deploy path. Assert `DeployResult.BundleName`, selected `Packages[].Name`, actual resources, and configuration effects. |
| Successful hooks | Observe package and bundle pre/post hooks during real successful deployment. Post hooks must run after success. Check dependency ordering without relying on sibling ordering. Preserve existing abort/error tests separately. |
| Custom loader success | Stage a real package via the public loader and complete deployment. Assert hook-visible layout definition/path, partial options where applicable, and observable mutations. |
| Deployed digest | Set a real resolved package digest through the public layout/hook. Confirm it is retained in externally observable deployed package identity, using public dependency APIs or Kubernetes state. Do not inspect private adapters. |
| Variables and configuration | Verify scalar/nested/list inputs produce expected deployed values/resources; check hook-visible operational options. Keep `Global` expectations limited to phase 2's compatibility behavior. |
| Remove success | Remove fixtures genuinely deployed from source, artifact, and OCI workflows. Assert `RemoveResult.BundleName`, package names/statuses, and resource disappearance. Preserve shared Zarf infrastructure. |
| Skipped removal | On an initialized cluster, remove an already-absent fixture. Assert `RemovePackageStatusSkipped == "skipped"` and success. Include a mixed removed/skipped result; assert the removed status constant too. |

**Phase exit:** real Deploy/Remove results are asserted; both infrastructure
modes pass; ownership/cleanup evidence exists; the public boundary guard passes.

#### Phase 3 evidence

- `TestClusterLifecycle`: real public bootstrap, owned-cluster deletion and
  preservation of existing clusters passed. Test process: 112.46 s; total: 139.51 s.
- Four `TestInCluster*` contracts passed: source Deploy/Remove; artifact deployment
  through a custom loader with a real registry digest; OCI Pull/Prepare/Deploy
  and Remove; mixed removed/skipped and all-absent removal. Test process: 24.11 s;
  total: 45.90 s. ConfigMaps prove scalar, nested and list values.
- Result membership, successful hook ordering, persisted public digest mutation,
  status constants and resource disappearance have explicit assertions.
- Missing explicit kubeconfig fails before cluster access. The supplied helper
  requires `KUBECONFIG` and `UDS_TEST_KUBECONFIG` to identify the same file.
- Post-run checks found no test namespaces or fixture package state. Zarf and
  the original `uds` cluster survived. Sol approved; scoped tagged lint passed.
- Evidence: `coverage/library-owned.*`, `coverage/library-in-cluster.*` under
  `/tmp/uds-cli-302-implementation/`. The first supplied attempt failed because
  a local artifact layout need not expose a registry digest. The fixture now
  obtains it through upstream package publication/reload; no production change.
- CI routing remains part of Phase 5's test-run split.

### Phase 4 — Credentialed keyless contracts

- [x] Establish a before-change credentialed baseline on an eligible CI runner.
- [x] Add direct public Sign → Verify success with `SigningModeKeyless`.
- [x] Add wrong-identity rejection and retain a small CLI flag-wiring check.
- [ ] Wire an explicit credentialed execution that cannot pass by skipping.

Before-change credentialed outcome: [run 35486324858](https://github.com/defenseunicorns/uds-cli/actions/runs/35486324858),
at the exact research revision, passed `TestKeylessSignVerify_Integration`
(1.09 s) and the package keyless verification subcase (2.93 s). The Next jobs
passed; the overall workflow did not. Credentialed coverage counters were not
collected, so this establishes behavior/timing, not a credentialed coverage floor.

The new `TestKeylessBundleSigning` deliberately fails without OIDC credentials.
Its trusted-CI success gate stays open until Phase 6/PR validation. Keep the old
CLI keyless execution in place until that evidence exists; other migrations may
proceed once their replacements pass.

Use [existing keyless integration](tests/integration/cmd/bundle/signing_integration_test.go)
to understand the GitHub OIDC prerequisites. Reuse the environment/identity
approach, not its Cobra calls or internal fixture helpers. Constrain certificate
identity and issuer; do not use an unrestricted identity expression.

The existing case requires `GITHUB_ACTIONS`, the Actions OIDC request variables,
repository/ref identity, and `id-token: write`. Reuse the existing eligible
credentialed execution and its permission policy. Missing identity must fail an
execution requiring this subset; ordinary local tests
must not claim keyless success after skipping it. Handle fork PRs explicitly:
report the credentialed check as unavailable when it cannot safely run, and
obtain trusted-run evidence before completion. Never expose OIDC tokens in logs.

Keep registry operations local where possible. Real keyless signing uses
external identity/transparency services; it is not part of the default local
subset. Classify it as Library Tests / non-cluster. Do not
dispatch publication/release workflows to obtain a test identity.

**Phase exit:** actual public signing and verification pass under the intended
identity; wrong identity fails; before/after evidence is retained.

#### Phase 4 local evidence

`TestKeylessPackageVerification` passed against the signed remote fixture,
including wrong-identity rejection and no output artifact (73.21 s test process).
Evidence: `coverage/library-package-keyless.*`. `TestKeylessBundleSigning` fails
immediately without GitHub OIDC; no skip masks missing credentials. Tagged lint,
boundary checks and Sol review passed. Trusted signing success and CI routing
remain open; the existing CLI keyless case is retained for that transition.

### Phase 5 — Move behavior down; retain CLI contracts

- [ ] Migrate the seven identified API-oriented scenarios after their lower-layer
  replacements pass and their coverage/unique assertions are accounted for.
- [ ] Move deep business assertions from other CLI scenarios into library tests.
- [ ] Complete the scenario inventory: every remaining CLI business assertion
  has a lower-layer replacement or a concrete CLI-specific reason to remain.
- [ ] Replace the slow Core build in the ordinary CLI non-cluster subset.
- [ ] Split CLI cluster bootstrap/lifecycle cases from supplied-cluster contracts
  and route them to the non-cluster and in-cluster runs respectively.
- [ ] Update Next tasks, CI, and contributor guidance together.

Initial migration list from
[Create integration](tests/integration/cmd/bundle/create_integration_test.go):

- `TestCreate_InitBundle`
- `TestCreate_SignatureVerification`
- `TestCreate_InitBundle_OptionalComponentIncluded`
- `TestCreate_InitBundle_OptionalComponentExcluded`
- `TestCreate_DefaultsConfig_IncludedAsOCILayer`
- `TestCreate_InitBundle_MultiArch`

Also migrate `TestPushPull_RoundTrip` from
[bundle integration](tests/integration/cmd/bundle/bundle_integration_test.go).
Replace internal fixture/assertion imports, not merely the build tag or directory.
Preserve optional-component and multi-architecture assertions. Package keyless
verification in Create is distinct from keyless bundle signing; preserve both
behaviors, including wrong-key/identity rejection. Prefer local signed fixtures;
put unavoidable external verification in an explicitly scheduled execution.

Keep `TestCreate_DefaultsConfig_Applied` as a CLI config-precedence check with a
small fixture. Preserve CLI flags, aliases, prompts, structured output, error
presentation, exit status, and Next-mode routing. API result-field assertions
must not replace JSON/YAML or process-output assertions.

`TestCreate_UDSCoreStandardBundle` took 706.43 seconds. Replace its short-job
Cobra coverage with a tiny local fixture. Check the three-group coverage floor
before deleting the old case. Do not add a second expensive Core build.

For every removed/moved assertion, record its old test, new owner, preserved
behavior, and coverage evidence. Keep representative CLI deploy/remove workflows
even where statement coverage overlaps library coverage.

Integration points:

- [tasks/tests.yaml](tasks/tests.yaml): retain existing task entrypoints; add
  narrowly scoped library executions and Next coverage collection as needed.
- Existing Next test invocations: divide suite selectors into the six runs above.
  Preserve build steps, artifact reuse, setup, caches, workflow dependencies,
  triggers, permissions, and release gating. Do not modify Legacy workflows.
- [CONTRIBUTING.md](CONTRIBUTING.md), [testing guidance](.agents/skills/testing/references/TESTING.md),
  and [AGENTS.md](AGENTS.md): synchronize changed Next workflows and boundaries.
  Use the existing build configuration; validate CLI behavior through `cmd/uds`
  with Next enabled, not only `cmd/uds-cli-next`.

Library jobs must not rely on a previously built UDS binary for fixtures.
Remove binary calls from test-owned library fixtures without changing shared CI
setup. Infrastructure fixtures may use k3d and existing public dependency APIs;
UDS operations stay public-only.

**Phase exit:** migrations preserve unique behavior and covered blocks; short
CLI default execution no longer builds full Core; all affected Next jobs/docs
agree on the six test runs while retaining existing CI logic and optimizations.
The boundary-based inventory satisfies unit > library > CLI. If it does not,
continue justified migrations and missing contracts; do not inflate counts or
delete unique coverage to make the diagram look right.

#### Phase 5 migration evidence

| Previous CLI case | Replacement or retained owner | Preserved behavior |
| --- | --- | --- |
| `TestCreate_InitBundle` | `TestCreateAndInspectPublicContract` | Public Create result, OCI layout/index/blob presence, and bundle-definition layer. |
| `TestCreate_SignatureVerification` public-key cases | `TestCreateInspectsSignedVerifiedPackage`, `TestCreateRejectsWrongPackageVerificationKey` | Signed/verified package summary; wrong key rejects Create before output. |
| `TestCreate_SignatureVerification` keyless cases | `TestKeylessPackageVerification` (`library,signing_integration`) | Keyless package verification success and wrong identity rejection before output. |
| `TestCreate_SignatureVerification` disabled verification warning | retained as `TestCreate_VerificationDisabledReportsUnverifiedPackage` | Cobra diagnostic includes `unverified package`, using a local fixture. |
| `TestCreate_InitBundle_OptionalComponentIncluded` | `TestCreateAndInspectPublicContract` | Prepared package actually contains `main` and `optional` components. |
| `TestCreate_InitBundle_OptionalComponentExcluded` | `TestCreateOptionalComponentCanBeExcluded` | Prepared package contains only `main`; optional component is absent. |
| `TestCreate_DefaultsConfig_IncludedAsOCILayer` | `TestCreateAndInspectPublicContract` | `defaults.uds.hcl` is an OCI layer. |
| `TestCreate_InitBundle_MultiArch` | `TestCreatePublicContractSupportsMultipleArchitectures` | `amd64`/`arm64` output paths and loaded upstream package metadata architectures. |
| `TestPushPull_RoundTrip` | `TestPushPullAndInspectPublicContract` | Local OCI Push/Pull result fields, original/OCI/pulled artifact digest, identical blob set and index bytes. |
| `TestReconfigure_CustomSuffix` | `TestReconfigureAddsDefaultsAndPreservesPackageManifests` | Public suffix output naming. |
| `TestReconfigure_InsertsDefaultsWhenOriginalHadNone` | `TestReconfigureAddsDefaultsAndPreservesPackageManifests` | Defaults layer added to an artifact without defaults. |
| OCI Reconfigure package-manifest block | `TestReconfigureAddsDefaultsAndPreservesPackageManifests` | Package manifest digests unchanged after Reconfigure. |
| `TestSignedOCIPull_Integration` | `TestSignVerifyOCIContract` | Signed OCI Pull rejects no policy with `ErrInvalidVerificationPolicy`; verified Pull succeeds. |


The nine former cluster CLI cases become six supplied-cluster cases plus one
owned lifecycle case. Artifact and OCI deploy/remove pairs retain their flags,
workload configuration, state secrets, structured results, and removal checks.
The remaining CLI sign/verify/inspect case uses a local artifact; tamper rejection
belongs to `TestSignVerifyPublicContract`. Neutral archive helpers preserve the
ADR-0007 bundle-definition artifact type when selecting layers and package
manifest digests. No library helper imports private UDS implementation packages.

Only the four existing Next job blocks changed in the reusable workflow. The
other job blocks, build, permissions, dependency graph, and Legacy invocations
are unchanged. Supplied clusters are provisioned outside the suites. Sol approved
the cluster split and provisioning helper; final artifact-migration review and
full coverage verification remain in progress.

### Phase 6 — Prove improvement and close the ticket

- [ ] Run the complete applicable Next matrix with fresh coverage.
- [ ] Publish per-execution and cumulative counts, outcomes, runtime, and profiles.
- [ ] Compare covered blocks for the default combination, required PR matrix,
  and full infrastructure matrix; explain every changed/deleted source block.
- [ ] Complete the exported-symbol-to-contract-test mapping.
- [ ] Publish before/after counts and percentages for the three layers, using
  the same counting rules. Include library/CLI cluster subcategories, the retained
  E2E scenarios and their purpose, and measured execution times.
- [ ] List newly covered production blocks and the consumer assertions that
  exercise them; distinguish new coverage from coverage moved between runners.
- [ ] Populate the ADR's “Test coverage after the changes” section with measured
  tables and a plain-text chart. Keep execution instructions in this plan.
- [ ] Resolve Sol's verified findings and record remaining external limitations.

## Completion gates: preserve coverage and improve the public contract

1. All contract rows in phases 2–4 have executable consumer assertions. No hook
   abort is counted as deployment success. No successful call is substituted for
   checking its public result fields or externally visible effects.
2. Library tests and helpers obey the public-only boundary in every tag variant.
3. Cumulative Next and public coverage do not decrease in any required comparison,
   and both strictly increase across the full applicable matrix. On unchanged
   research source, exceed **4,953 / 5,982 Next statements** and **983 / 1,308 public
   statements**. Compare exact fractions, not rounded percentages. Preserve every
   previously covered unchanged statement; gains elsewhere do not excuse losses.
   Use matched before/after baselines when source, platform, or credentialed
   executions differ. Moving an already-covered block to library tests alone
   does not satisfy cumulative improvement.
4. Default-library public coverage strictly exceeds **390 / 1,308 (29.82%)**
   on comparable source, with new meaningful assertions. Report cluster/signing
   library coverage separately and as part of the library union.
5. Constants, type shape, aliases, error identity, ownership, and hooks are
   protected explicitly; statement percentages alone cannot establish them.
6. The boundary-based scenario counts satisfy **unit > library > CLI**. Library
   contracts grow and CLI business duplication shrinks. Record the small retained
   CLI E2E set and each workflow's distinct purpose. Report counts and percentages,
   including cluster subcategories, using Phase 1's rules. Do not split tests to
   manufacture a ratio or delete unique assertions just to reduce counts.
7. Required credentialed/cluster execution cannot be replaced with a green skip.
   Preserve failing/unavailable results honestly; do not lower thresholds or
   exclude relevant production packages to manufacture improvement.
8. Next tests are divided into the six agreed runs. Existing build and workflow
   logic, shared artifacts, caches, setup, and other CI optimizations are preserved.
   Do not add sequencing between test groups or change Legacy workflows. Phase 6
   verifies cumulative coverage from the collected evidence.

## Coverage measurement requirements

Keep execution instructions and evidence here or in contributor tooling, not in
the ADR's concise current-state section.

1. Freeze the production package list under `cmd/`, `internal/`, and `pkg/` for
   Next; exclude `/legacy/` packages and test helpers. The public subset is
   `pkg/bundle`, `pkg/bundle/spec`, and `pkg/iostreams`. Include zero-hit code.
2. Run the chosen Next unit packages explicitly. Do not use a broad repository
   test command that also executes the Old suite. Use the same source revision,
   Go version, OS/architecture, tags, and instrumentation for baseline/final.
3. Use fresh runs (`-count=1`), atomic coverage, and the same `-coverpkg` set for
   test processes and an instrumented primary `cmd/uds` binary. Preserve JSON
   outcomes and the actual process exit status, including setup/teardown failure.
4. Provide the instrumented binary via `UDS_CLI_PATH` only for the minimal
   process-entrypoint checks, which select Next mode explicitly.
   Preserve required build flags from the repository build task; in the research
   run, the embedded Zarf command prefix and version were also supplied.
5. Give each execution a fresh child `GOCOVERDIR`. Go test sets a coverage
   environment for its own process; explicitly propagate the intended directory
   to subprocesses. The research used an `-exec` wrapper to restore the child
   directory from a separate environment variable. Confirm child counter files
   exist; a test-process profile alone misses executed CLI code.
6. Convert child coverage with Go's `covdata` tooling and union it with the
   matching test-process profile. Deduplicate blocks by source location and
   statement count, OR-ing hit status even within one profile. Never let a later
   zero counter overwrite an earlier hit.
7. For each fixed scope, divide covered statement count by total statements.
   Cumulative coverage unions hit blocks; do not sum percentages or average
   package percentages. Cross-check the resulting profile with `go tool cover`.
8. Keep all raw profiles/logs and a compact manifest: commit, toolchain/platform,
   package list, tags, instrumentation, outcomes, elapsed times, and any fixture
   or cluster overrides. If production changes, report comparable-source and
   full-source denominators, with a mapping for changed/deleted blocks.

These are measurement requirements, not a request to build a generalized
coverage service. Reuse repository tools first; add only the smallest reproducible
collection/check needed by Next CI. Keep credentials and machine-local kubeconfig
contents out of retained review artifacts.

## Follow-up after CLI-302: fuzzing

- [ ] Schedule one bounded native Go fuzz target after phase 6.
- [ ] Choose private HCL parsing in units or public domain validation through
  `pkg/bundle/spec`; keep deterministic seeds in ordinary test runs.
- [ ] Retain any discovered regression; run fuzz campaigns separately.

Do not fuzz live deployments, registries, or signing services. Fuzzing does not
replace any required API contract or block delivery of the focused CLI-302 work.

## Final CLI execution and guidance revision

- Execute CLI command contracts directly through a fresh Cobra root and IOStreams.
- Return errors from Next command handlers with RunE. Report and exit only at the executable boundary.
- Preserve a minimal primary-binary check for process initialization, Next-mode routing, Zarf aliases, and exit behavior.
- Keep provisioning outside supplied-cluster suites. Owned-cluster tests retain exact-name cleanup.
- Use standalone Zarf matching go.mod for package action callbacks in owned/Core tests. Cache it with the existing pinned mise-action only in the Next jobs that need it, and activate it through mise exec. Leave global tool configuration and Legacy setup unchanged. Do not build another test executable or duplicate main.
- Serialize direct command calls while logging remains process-global; retain independent test jobs and parallel resource assertions. Measure the resulting runtime.
- Review AGENTS.md and every repository AI skill/reference against ADR 0027 and the final selectors. Preserve Legacy guidance and historical ADRs.

### Final validation notes

- Boundary inventory: 485 unit, 70 library, 67 CLI scenarios; one example is separate. Infrastructure-helper checks are excluded.
- Direct Cobra preserves error returns, stderr ownership, process-global settings, and source-relative removal cleanup. The minimal binary check covers startup routing, Zarf aliases/features, and process errors.
- All four public in-cluster contracts passed against a cluster created and deleted by the external provisioner using Zarf 0.86.0.
- Default library, unit, direct CLI, supplied CLI, and owned CLI runs passed. Only passing runs contribute to final coverage.
- Two Core measurements failed on GHCR HTTP/2 protocol errors before deployment. The subsequent measurement uses HTTP/1.1 with unchanged TLS and workload assertions.
- One local lifecycle measurement detected the concurrent teardown of another measurement's cluster. The subsequent isolated lifecycle run passed; CI jobs use separate runners.
- AI guidance reviewed: AGENTS.md and all three repository skill packages. Updated testing and Go guidance; documentation-authoring guidance already fits the new structure.
