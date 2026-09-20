# Testing Guide

UDS CLI testing strategy. For generic Go testing rules, see [the Go development reference](../../go-development/references/GO_CODE_STANDARDS.md#18-testing).

## Principles

- [ADR-0027](../../../../docs/adr/0027-public-api-test-pyramid.md) defines three
  layers: unit, public library, and CLI. Cluster ownership is an execution
  purpose, not an extra layer.
- Maintain a healthy testing pyramid: many unit tests, fewer Library tests,
  and the fewest CLI tests. Push each behavior to the lowest layer that can
  prove it: unit tests for underlying logic, Library tests for public API
  contracts, and CLI tests for command-line behavior. Do not add or move tests
  merely to change counts.
- Reproduce bugs as close to the end-user experience as practical before fixing.
- Prefer fast unit tests for pure logic and validation.
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
uds run test:next-unit
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
- Prefer focused Legacy E2E tasks from `uds run --list-all` and run only the task needed for the behavior under test.
- Ask for explicit approval before running any GHCR-writing task.

### Next CLI tests

- Location: `tests/cli/...`.
- Build tag: `cli`.
- Scope: command-line arguments, flags, aliases, config precedence, prompts,
  and user-facing output.
- Execute commands through a fresh Cobra root and `iostreams`. Next handlers
  return errors with `RunE`; command tests assert the returned error rather than
  a process exit.
- Keep primary `cmd/uds` execution to the minimal process-only checks: Zarf
  passthrough, Next-mode routing, and error/exit behavior. Do not duplicate
  library contracts through the binary.
- These tests should not own deep business logic coverage when unit or library tests can cover it.
- Run with:

```bash
uds run build
uds run test:next-cli-non-cluster
```

### Next Library tests

- Location: `tests/library/...`.
- Build tag: `library`.
- Scope: public library behavior, especially `pkg/bundle`, `pkg/bundle/spec`, and `pkg/iostreams` APIs, hooks, options, results, and public error contracts.
- Library tests and every fixture or assertion helper they transitively import
  must use only those public UDS packages. Do not call internal UDS packages,
  Cobra, or a UDS CLI binary.
- Run with:

```bash
uds run test:next-library-non-cluster
```

The credentialed subsets use `library,signing_integration`:

```bash
uds run test:next-library-non-cluster-package-verification
uds run test:next-library-non-cluster-keyless
```

The first verifies signed remote packages. The second requires GitHub Actions
OIDC credentials and must fail when they are unavailable.

### Cluster execution

Both Library and CLI tests have two subcategories: in-cluster and non-cluster.
These describe cluster requirements; they do not add layers to the pyramid.

#### In-cluster

- Library and CLI in-cluster tests require `KUBECONFIG` and
  `UDS_TEST_KUBECONFIG` to name the same explicit Zarf-ready kubeconfig.
- Provision that cluster outside the suite with `hack/test-cluster.sh`. The
  suite never adopts, creates, initializes, or deletes the supplied cluster;
  it cleans up only its test-owned resources.
- Run with the nearest task selector:

```bash
uds run test:next-library-in-cluster
uds run test:next-cli-in-cluster
```

#### Non-cluster

- Tests start with no supplied cluster and may create a uniquely named,
  isolated cluster. They use an isolated kubeconfig and remove only resources
  and clusters created by that execution.
- Library lifecycle setup still uses public UDS APIs; infrastructure creation
  never changes a library test into a Cobra test.
- Ask before running tests that create or mutate a cluster.

## IOStreams pattern

Use `iostreams.NewTestIOStreams()` when testing code that writes command output or diagnostics.

```go
streams, _, out, errOut := iostreams.NewTestIOStreams()
_ = out
_ = errOut

// Pass streams into options, commands, or public APIs that accept IOStreams.
```

When a direct Cobra test triggers a Zarf package-action callback, use the
mise-managed standalone Zarf version matching `go.mod`; do not substitute a UDS
binary.

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

- Testing Cobra parsing in unit tests when a CLI test is more appropriate.
- Testing deep business logic only through CLI tests.
- Mixing Legacy and Next fixtures or assertions without a migration-specific reason.
- Running cluster, GHCR, or destructive tests without approval.
- Leaving generated artifacts, bundles, or cluster resources behind.
