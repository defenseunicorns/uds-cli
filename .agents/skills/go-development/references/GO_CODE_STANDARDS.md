# Go Coding Guidelines

Distilled from Effective Go, Google Go Style, Uber Go Style, Go Code Review Comments, and this repository's ADRs. Applies to every `.go` file in this repo.

These rules are self-contained. Do not fetch external sources at runtime.

## 1. Style Core

- Format Go code with `gofmt` and `goimports`. Never hand-format.
- Tabs for indentation. There is no hard line length cap, but reduce nesting before wrapping.
- Favor early returns and guard clauses to avoid deep nesting; keep the happy path at the top level.
- Group related declarations with `var (...)` or `const (...)` blocks.
- Keep one concept per file. Split large files when they accumulate unrelated types or responsibilities.
- Prefer readable, direct code over clever abstractions.

## 2. Naming

- Packages: short, lowercase, single word, no underscores or camelCase. Avoid `utils`, `common`, `helpers`, and `models`.
- Files: `snake_case.go`. Test files end in `_test.go`.
- Exported identifiers: `MixedCaps`. Unexported identifiers: `mixedCaps`.
- Acronyms keep their case: `ID`, `URL`, `HTTP`, `IO`, `OCI`, `JSON`.
- Constructors: `New` when the package exports one primary type, `NewThing` otherwise.
- Errors: sentinel values are prefixed `Err`; error types are suffixed `Error`.
- Booleans should read as predicates: `isReady`, `hasValue`, `enabled`. Avoid negative names.
- Receivers should be short, usually one or two letters, and consistent across methods of the same type.
- Getters drop the `Get` prefix. Setters keep `Set`.
- Single-method interfaces usually use the method name plus `-er`, such as `Reader` or `Deployer`.

## 3. Architecture

- Follow relevant ADRs in `docs/adr/` before designing or changing code.
- `pkg/` contains supported public library packages.
- `internal/` contains repository-private implementation packages.
- Cobra command wiring belongs in `internal/cli`; business logic belongs in `pkg/` or sibling `internal/` packages.
- Do not expose `internal/logger`, `internal/printer`, `internal/version`, or private implementation types in exported `pkg/` signatures.
- Keep Legacy and Next boundaries separate:
  - Legacy: `internal/legacy/...`, `pkg/legacy/...`, `tests/legacy/e2e`, `testdata/legacy`.
  - Next: canonical `internal/...` excluding `internal/legacy`, `pkg/bundle/...`, `pkg/iostreams`, `tests/integration`, `tests/library`, `tests/cluster`, `tests/smoke`.
- New feature work targets Next unless a maintainer explicitly scopes the change to Legacy.
- Do not make Legacy packages depend on canonical Next packages.

### Zarf and OCI operation ownership

Prefer upstream Zarf APIs for Zarf package behavior. Use this repo's `internal/oci` for UDS bundle artifact behavior that Zarf does not own.

Use upstream Zarf libraries when operating on Zarf packages, including:

- Loading or inspecting local Zarf packages.
- Reading or interpreting `zarf.yaml`.
- Inspecting package metadata, components, images, variables, constants, or manifests when Zarf exposes an API.
- Package deploy, remove, init, and related lifecycle operations.
- Package layer, archive, and layout handling.
- Package signature verification and verification material handling.
- Pulling, publishing, copying, or resolving package content when the object is a Zarf package.

Use this repo's `internal/oci` when operating on UDS bundle artifacts, including:

- UDS bundle artifact layout, root indexes, and bundle manifests.
- Bundle HCL, defaults, values, and other bundle-owned layers.
- Bundle signatures and bundle signature evidence.
- Bundle push, pull, tag, and local OCI store behavior when the object is a UDS bundle artifact.
- Generic OCI helpers needed by bundle code that Zarf does not provide.

Avoid:

- Calling ORAS directly outside `internal/oci`.
- Reimplementing Zarf package layout parsing when Zarf exposes package or layout APIs.
- Reading Zarf package manifests through generic OCI APIs only to duplicate upstream Zarf behavior.
- Adding new OCI traversal logic without first checking whether Zarf or `internal/oci` already owns it.

Examples:

- Need metadata from a Zarf package? Start with upstream Zarf package or layout APIs.
- Need to stage or copy Zarf package content for bundle assembly? Prefer upstream Zarf package pull, copy, archive, or layout APIs when they expose the needed behavior.
- Need to read or write the UDS bundle root index? Use `internal/oci`.
- Need to fetch bundle HCL or bundle signature layers? Use `internal/oci` or the bundle package that wraps it.
- Need command behavior that deploys packages? Use `internal/zarf` and upstream Zarf APIs; keep cobra wiring in `internal/cli`.

## 4. Declarations

- Inside functions, prefer `:=` for new variables.
- Use `var x T` for zero-value initialization or when the type matters.
- Watch shadowing. Use `if err := f(); err != nil` to scope an error intentionally.
- Use explicit constants when stable numeric values matter. Use `iota` only for closed internal enums where insertion or reordering cannot break persisted or external values.
- Use `const` for compile-time values and `var` for runtime defaults.

## 5. Control Flow

- Return early on errors and completed branches. Do not wrap the happy path in `else` after a return.
- Prefer `switch` over chained `if/else` when branching on one value.
- `for` is the only loop keyword. Use `for { ... }` for infinite loops and `for range` for collections.
- Avoid labels and `goto` outside generated code.

## 6. Functions

- Prefer exported declarations before unexported declarations when practical.
- Group methods by receiver type.
- Return `(value, error)` rather than a struct that only wraps both.
- Use named returns only when they document results or simplify a deferred mutation.
- Avoid naked returns in long functions.
- Variadic args are rare. Prefer slices unless variadic form improves the API.

## 7. Errors

- Return errors. Do not panic except in `main` initialization or truly unrecoverable invariants.
- Wrap with context using `%w`: `fmt.Errorf("read bundle %s: %w", path, err)`.
- Compare with `errors.Is` and unwrap with `errors.As`.
- Define package-level sentinels in the owning package: `var ErrNotFound = errors.New("not found")`.
- Define typed errors when callers need to inspect fields. Implement `Error() string` and `Unwrap()` or `Is(target error) bool` when needed.
- Combine multiple errors with `errors.Join(...)`.
- Never log and return the same error. Pick one. The top-level caller logs once.
- Do not capitalize error message strings or end them with punctuation. Acronyms and proper nouns keep their natural case.

### Next error ownership

- There is no repository-wide error package.
- Each package that declares sentinel errors or named error types places those declarations in that package's `errors.go`.
- Methods belonging to named error types, including `Error`, `Unwrap`, and `Is`, also belong in the owning package's `errors.go`.
- Operation-specific error creation and wrapping stay at the call site in the file that performs the operation.
- `errors.go` is not a collection of every error message returned by the package.
- Internal packages must not import `pkg/bundle/` for public error declarations.
- When callers need stable `errors.Is` or `errors.As` behavior, the facade maps internal failure to a sentinel or error type declared in the public package.

## 8. Context

- `context.Context` is always the first parameter, named `ctx`.
- Never store a context in a struct. Pass it through the call chain.
- Never pass a nil context. Use `context.Background()` at process entry. Avoid `context.TODO()` in production code; use it only as a short-lived placeholder with a comment explaining what context should replace it.
- Use `context.WithTimeout` or `context.WithCancel` and always `defer cancel()`.
- Do not use `context.Value` for required arguments.

## 9. Concurrency

- A goroutine you start is yours to clean up. Document its exit condition.
- Channels are for ownership transfer. Mutexes are for protecting state. Default to mutexes when in doubt.
- The sender closes a channel, never the receiver.
- Do not close a channel with multiple senders without synchronization.
- Use `errgroup.Group` for related goroutines that need cancellation and a single error.
- Use `sync.Once` for one-shot initialization.
- Run `go test -race` on code touching shared state.

## 10. Data Structures

- Preallocate slices with `make([]T, 0, n)` when the size is known.
- Copy slices or maps before handing them to outside callers if mutation would violate the contract.
- Maps are not safe for concurrent writes. Use `sync.RWMutex` or `sync.Map` only when the access pattern fits.
- Design types so their zero state is useful when practical.
- Use `time.Time` for timestamps. Compare with `time.Equal`, not `==`.

## 11. Interfaces

- Accept interfaces and return structs.
- Define interfaces on the consumer side, where they are used.
- Keep interfaces small. One or two methods is normal.
- Add compile-time checks near implementations: `var _ Deployer = &ZarfDeployer{}`.
- Embed interfaces to compose larger interfaces. Embed structs sparingly.

## 12. Generics

- Use generics when they remove real duplication without losing readability.
- Constrain on behavior when possible.
- Do not generic-ify single-use code.

## 13. Functional Options

Use functional options for constructors with many optional parameters or defaults that vary by call site.

```go
type Option func(*Server)

func WithTimeout(d time.Duration) Option {
	return func(s *Server) { s.timeout = d }
}

func NewServer(addr string, opts ...Option) *Server { ... }
```

## 14. Defensive Coding

- Validate inputs at API boundaries and return descriptive errors.
- Public methods in `pkg/bundle/` validate their options structs as the first statement.
- Use `defer` to release resources. Check close errors when they matter, especially writes.
- For dangerous primitives, expose a safe API and use `MustX` only for package initialization or invariant setup.
- Never modify a slice or map received from a caller unless the contract says you may.
- Do not log secrets, tokens, credentials, kubeconfigs, or full file contents.

## 15. Logging and Output

- Follow ADR guidance for stdout, stderr, logging, and result objects, especially [ADR-0004](../../../../docs/adr/0004-logging-and-output-strategy.md) and [ADR-0012](../../../../docs/adr/0012-library-logging-via-iostreams.md).
- Prefer structured logging through the repository's logging abstractions.
- CLI command user output and machine-readable results should stay separate from diagnostic logs.
- Do not leak logging or printer implementation details into public `pkg/` APIs unless an ADR explicitly allows it.

## 16. Performance

- Measure before optimizing. Use `go test -bench`, `pprof`, and `benchstat` when performance matters.
- Prefer `strconv` over `fmt` for hot-path conversions.
- Use `strings.Builder` for repeated string concatenation and `bytes.Buffer` for byte streams.
- Use `sync.Pool` only when profiling shows allocation pressure.

## 17. Documentation

- Every exported identifier has a doc comment.
- The first sentence starts with the identifier name and reads as a complete sentence.
- Package docs go in `doc.go` or at the top of the file with the package's main type.
- Runnable examples in `example_test.go` files double as tests and docs.
- If logic changes user-visible behavior, update relevant docs in `docs/` or generated CLI docs.
- If contributor flow changes, update `CONTRIBUTING.md` and/or `README.md`.

## 18. Testing

See [the testing skill](../../testing/references/TESTING.md) for the project-specific testing strategy. Highlights:

- Table-driven tests with subtests are preferred for related cases.
- Use `testify/assert` for soft assertions and `testify/require` when continuing would be misleading.
- Mark helpers with `t.Helper()`.
- Use `t.Cleanup` over `defer` for test teardown.
- Use `t.Parallel()` only for independent tests that do not share global state.

## 19. Linting

- Start with `hk fix --all` when you want the repository hooks to apply supported automatic fixes.
- Format Go files before linting. Use `gofmt` for formatting and `goimports` for import grouping when hooks do not handle it.
- Run `uds run lint` after formatting to check the repository lint rules.
- Run `hk check --all` to run the configured local hook checks without applying fixes.
- Suppress lint findings narrowly with `//nolint:linter // reason`. Always include a reason.
- Do not disable a linter globally to silence a single case.

## 20. Dependency Management

- Add dependencies only when the functionality is non-trivial to write and the upstream is well-maintained.
- Run `go mod tidy` after dependency changes.
- Keep `vendor/` consistent when dependency changes require it.
- Pin GitHub Actions to specific versions. Renovate watches configured dependency locations. Add custom managers for new dependency files when needed.
