# 20. Repository Package Structure

Date: 2026-08-07

## Status

Approved

If accepted, this ADR will supersede the package-placement portions of
[ADR-0002](0002-cli-architecture-patterns.md) and the command and printer
package-placement references in
[ADR-0004](0004-logging-and-output-strategy.md). ADR-0002's command pattern,
IOStreams abstraction, cobra/business-logic separation, and testing principles
remain in force. ADR-0004's stdout/stderr separation, result-object pattern, and
output-formatting behavior also remain in force.

## Context

The bundle implementation currently shares a package with its supported Go API.
This makes it difficult to distinguish the compatibility surface available to
external consumers from implementation details that may change as bundle
formats, storage, and deployment behavior evolve.

ADR-0002 also places cobra command wiring in `pkg/cmd/`. Packages under `pkg/`
are conventionally understood to be importable library packages, but command
wiring is an application concern and is not a supported library API.

Go's `internal` directory rule provides the boundary needed for implementation
packages. Placing these packages at the repository root allows any package in
this module to reuse them while preventing consumers outside this module from
importing them.

## Decision

Top-level directories define the support boundary:

- `pkg/` contains supported Go library packages. The public bundle API lives in
  `pkg/bundle/` and remains free of cobra dependencies.
- `internal/` contains repository-private implementation packages. Bundle HCL,
  artifact, OCI, Zarf, logging, printing, and version implementation code will
  live beneath this directory.
- `cmd/` contains executable entrypoints.
- `internal/cli/` contains cobra command wiring. Existing command code in
  `pkg/cmd/` will move to `internal/cli/`.

The bundle-focused target structure is:

```text
cmd/                                      # Executable entrypoints
  `-- uds-cli-next/
      `-- main.go
internal/                                 # Implementation hidden from external consumers
  |-- cli/                                # Cobra command wiring
  |   |-- cmd.go                          # Root command
  |   |-- bundle/                         # Bundle subcommands
  |   |-- config/                         # Configuration subcommands
  |   |-- tools/                          # Tool subcommands
  |   |-- util/                           # Command-only utilities
  |   |-- version/                        # Version subcommands
  |   `-- zarf/                           # Zarf subcommands
  |-- logger/                             # Structured console logging
  |-- printer/                            # CLI command-result formatting
  |-- version/                            # Build-time version information
  |-- bundle/                             # Bundle parsing, configuration, validation, and dependency logic
  |-- artifact/                           # Bundle archive lifecycle
  |-- oci/                                # Bundle OCI storage and transport
  |-- zarf/                               # Private Zarf SDK integration
  `-- testutil/                           # Shared repository-private test fixtures
pkg/                                      # Supported Go library packages
  |-- bundle/                             # Public UDS bundle API
  |   |-- doc.go                          # Package documentation and usage examples
  |   |-- types.go                        # Public options, results, hooks, and settings
  |   |-- errors.go                       # Public sentinel and typed errors
  |   |-- validation.go                   # Public operation-option validation
  |   |-- ...                             # Non ADR bound implementation details
  |   `-- spec/                           # Shared public bundle domain model
  |       |-- types.go                    # Bundle, package, metadata, and reference types
  |       `-- validation.go               # Bundle domain validation
  `-- iostreams/                          # Standard I/O stream abstractions
```

Test files remain colocated with the code they exercise and are omitted from the
diagram.

Error ownership remains package-local; there is no repository-wide error package.
Each package that declares package-level sentinel errors or named error types
will place those declarations in that package's `errors.go`, such as
`pkg/bundle/errors.go`, `internal/bundle/errors.go`, or
`internal/zarf/errors.go`. Methods belonging to named error types, including
`Error`, `Unwrap`, and `Is`, also belong in the owning package's `errors.go`.
Operation-specific error creation and wrapping remain at the call site in the
file that performs the operation; `errors.go` is not a collection of every error
message returned by the package. Internal packages must not import `pkg/bundle/`
for public error declarations. When callers need stable `errors.Is` or
`errors.As` behavior, the facade maps the internal failure to a sentinel or
error type declared in `pkg/bundle/errors.go`. Other internal errors may retain
contextual wrapping without joining the supported public error contract.

```text
cmd/uds-cli-next
  -> internal/cli
      -> pkg/bundle
          -> internal/{bundle,artifact,oci,zarf,logger}
          -> pkg/bundle/spec
      -> internal/{logger,printer,version}

internal/{bundle,artifact,zarf}
  -> pkg/bundle/spec
```

- `cmd/uds-cli-next/` may import `internal/cli/`; no other production package may
  depend on `internal/cli/`.
- `internal/cli/` calls supported APIs under `pkg/` and does not implement bundle
  business logic.
- `pkg/bundle/` provides the supported facade over bundle implementation
  packages under `internal/`.
- Bundle implementation packages under `internal/` must not import
  `pkg/bundle/`. They may import `pkg/bundle/spec/` for the shared bundle domain
  model. Public options, results, hooks, settings, and errors remain owned by the
  facade and are adapted to private implementation types where necessary.
- `internal/logger/`, `internal/printer/`, and `internal/version/` must not
  appear in exported `pkg/` signatures.
- `pkg/bundle/spec/` is the canonical owner of bundle domain types that require
  shared type identity, including bundles, packages, metadata, and package
  references. The package must not import `pkg/bundle/` or packages under
  `internal/`.
- New public APIs use `pkg/bundle/spec/` types. `pkg/bundle/` may alias a spec
  type to preserve an established API or provide a documented `bundle.Type`
  facade. Each alias must document its rationale.
- `internal/bundle/` owns private decoding types containing HCL parser state
  and converts them to `pkg/bundle/spec/` types when parsing completes. Public
  consumers and downstream internal implementations then operate on the same
  semantic model without further conversion.
- Behavior intrinsic to the domain model belongs to `pkg/bundle/spec/`.
  Bundle validation remains a `Validate` method on `spec.UDSBundle`. Deployment,
  inspection, formatting, IO, and implementation-specific behavior remain in
  `pkg/bundle/` or the relevant internal package. For example,
  `bundle.ToInspectResult` accepts a `*spec.UDSBundle` argument.
- `pkg/iostreams/` is a shared public dependency that may be imported by
  `pkg/bundle/`, `internal/cli/`, and other internal implementation packages.
- Public methods in `pkg/bundle/` continue to validate their option structs as
  their first statement, as required by
  [ADR-0011](0011-validation-at-public-library-entrypoints.md).

## Consequences

### Positive

- Go enforces that external modules cannot import implementation packages.
- Public consumers and private implementations use the same bundle domain type,
  preserving pointer identity and hook mutations without conversion or
  synchronization code.
- HCL parser state remains private and can evolve without changing the supported
  bundle domain model.
- The module can reuse root-level internal packages without exposing them to
  external consumers.
- Cobra wiring, logging, output formatting, and build metadata remain private.

### Negative

- The migration changes imports from `pkg/cmd/`, `pkg/logger/`, `pkg/printer/`,
  `pkg/version/`, and implementation code in `pkg/bundle/`.
- Changes to types in `pkg/bundle/spec/` must preserve the compatibility policy
  for supported APIs.
- Parsing requires an explicit conversion from private HCL decoding types to the
  public semantic model.
- Existing consumers that refer to domain types through `pkg/bundle/` must update
  their imports unless a documented compatibility alias is provided.
- Root-level `internal/` provides a broader module-internal visibility boundary
  than `pkg/bundle/internal/`; architecture and review must prevent unrelated
  packages from bypassing `pkg/bundle/` without a deliberate reason.

### Neutral

- The executable remains at `cmd/uds-cli-next/main.go`; command wiring moves
  from `pkg/cmd/` to `internal/cli/`.
- `pkg/iostreams/` remains a supported package because its `IOStreams` type is
  part of the public bundle API established by
  [ADR-0012](0012-library-logging-via-iostreams.md).
- The reorganization does not change CLI behavior.

## Alternatives Considered

### Keep implementation in `pkg/bundle`

Rejected because Go cannot enforce a boundary between the supported bundle API
and implementation details when both occupy the same package.

### Use `pkg/bundle/internal`

Rejected because its packages would only be importable by code beneath
`pkg/bundle`. Root-level `internal/` preserves external encapsulation while
allowing deliberate reuse elsewhere in this module.

### Keep command wiring in `pkg/cmd`

Rejected because cobra wiring is part of the CLI application, not a supported
Go library. Keeping it under `pkg/` obscures that distinction.

### Duplicate public and private bundle domain types

Rejected because bundle and package values cross public hooks and are mutable.
Maintaining separate public and private representations would require conversion
and synchronization around those hooks, add opportunities for state to diverge,
and lose shared pointer identity without providing a concrete compatibility
benefit for the bundle domain model.

### Automatically alias spec types from `pkg/bundle`

Rejected because it gives each domain type two public names, obscures ownership,
and prevents `pkg/bundle/` from defining methods on those aliases. Direct imports
from `pkg/bundle/spec/` are the default; narrowly scoped aliases remain available
for documented compatibility migrations or an intentional ergonomic
`bundle.Type` facade API.

### Keep logger, printer, and version under `pkg`

Rejected because these packages support the CLI and bundle implementation but
are not required as direct imports by bundle library consumers. Keeping them
under `pkg/` would imply a compatibility commitment without a supported public
use case.

### Move `iostreams` under `internal`

Rejected because exported bundle options contain `iostreams.IOStreams`.
External consumers could not use those options if Go prohibited them from
importing the defining package.

## References

- [ADR-0002 - CLI Architecture Patterns](0002-cli-architecture-patterns.md)
- [ADR-0004 - Logging and Output Strategy](0004-logging-and-output-strategy.md)
- [ADR-0011 - Validation at Public Library Entrypoints](0011-validation-at-public-library-entrypoints.md)
- [ADR-0012 - Library Logging via IOStreams](0012-library-logging-via-iostreams.md)
- [Go 1.4 Release Notes: Internal Packages](https://go.dev/doc/go1.4#internalpackages)
