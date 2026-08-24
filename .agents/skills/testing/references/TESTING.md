# Testing Guide

UDS CLI testing strategy. For generic Go testing rules, see [the Go development reference](../../go-development/references/GO_CODE_STANDARDS.md#18-testing).

## Principles

- Reproduce bugs as close to the end-user experience as practical before fixing.
- Use the lowest test tier that proves the behavior.
- Prefer fast unit tests for pure logic and validation.
- Use integration tests for wiring, public API behavior, or binary behavior that unit tests cannot prove.
- Ask before running cluster tests, GHCR-writing tests, destructive tests, or tests that mutate shared state.
- Keep Legacy and Next tests in their own lanes.

## Layers

### Unit tests

- Location: `*_test.go` next to the code.
- Packages: usually the same package as the code when internal access is useful.
- Scope: individual functions, methods, validation, small orchestration units.
- Avoid real cluster, registry, or CLI binary dependencies.
- Run with:

```bash
uds run test:unit
uds run test
```

`uds run test:unit` runs package unit tests. `uds run test` currently runs preparation checks, including unit tests and architecture checks.

### Architecture checks

- Verifies Legacy packages do not depend on canonical Next packages.
- Run with:

```bash
uds run test:architecture
```

### Legacy E2E tests

- Location: `tests/legacy/e2e`.
- Fixtures: `testdata/legacy`.
- Scope: Legacy CLI behavior and compatibility.
- Build `build/uds` before running tests that drive the binary.
- Prefer focused Legacy E2E tasks from `uds run --list-all` instead of the full suite when possible.
- Run all Legacy E2E with:

```bash
uds run test:legacy:e2e
```

Some Legacy tasks write to GHCR or require specific cluster state. Ask before running those.

### Next command integration tests

- Location: `tests/integration/...`.
- Build tag: `integration`.
- Scope: cluster-free CLI command wiring and binary behavior.
- These tests should not own deep business logic coverage when unit or library tests can cover it.
- Build `build/uds` first because tests use `UDS_CLI_PATH`.
- Run with:

```bash
uds run build
uds run test:next-integration
```

### Next library integration tests

- Location: `tests/library/...`.
- Build tag: `library`.
- Scope: public library behavior, especially `pkg/bundle` APIs, hooks, options, results, and public error contracts.
- No cluster, registry, or CLI binary should be required unless the test clearly documents why.
- Run with:

```bash
uds run test:next-integration-library
```

### Next cluster integration tests

- Location: `tests/cluster/...`.
- Build tag: `cluster_integration`.
- Scope: Next behavior against k3d or a live Kubernetes cluster.
- Build `build/uds` first.
- Ask for approval before running because these tests create and mutate cluster resources.
- Run with:

```bash
uds run build
uds run test:next-cluster
```

### Next UDS Core smoke tests

- Location: `tests/smoke/...`.
- Build tag: `uds_core_smoke`.
- Scope: live-cluster UDS Core smoke coverage for Next.
- Intended for release, nightly, or explicit validation rather than normal local loops.
- Build `build/uds` first.
- Ask for approval before running.
- Run with:

```bash
uds run build
uds run test:next-smoke-uds-core
```

## IOStreams pattern

Use `iostreams.NewTestIOStreams()` when testing code that writes command output or diagnostics.

```go
streams, _, out, errOut := iostreams.NewTestIOStreams()
_ = out
_ = errOut

// Pass streams into options, commands, or public APIs that accept IOStreams.
```

## Assertions

- Use `require` when a failed assertion means the rest of the test cannot continue safely.
- Use `assert` for additional independent checks after required setup succeeds.
- Mark test helpers with `t.Helper()`.
- Use `t.Cleanup` for cleanup so teardown is associated with the test lifecycle.
- Use `t.Parallel()` only for tests that do not share global state, environment variables, filesystem paths, clusters, registries, or package-level settings.

## Error testing

- For public Next APIs, test stable error contracts with `errors.Is` and `errors.As`, not only string matching.
- String matching is acceptable for user-facing CLI output when the text is the behavior under test.
- Internal error tests may assert package-local sentinel or typed errors.

## Documentation expectations

- If a test documents new user-visible behavior, ensure corresponding docs in `docs/` or generated CLI docs are updated.
- If a test changes contributor commands, setup, hooks, or local workflows, update `CONTRIBUTING.md` and/or `README.md`.

## Avoid

- Testing cobra parsing in unit tests when a command integration test is more appropriate.
- Testing deep business logic only through CLI integration tests.
- Mixing Legacy and Next fixtures or assertions without a migration-specific reason.
- Running cluster, GHCR, or destructive tests without approval.
- Leaving generated artifacts, bundles, or cluster resources behind.
