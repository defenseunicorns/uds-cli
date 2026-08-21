# 2. CLI Architecture Patterns

Date: 2026-02-03

## Status

Accepted. Partially amended by [ADR-0011](0011-validation-at-public-library-entrypoints.md), which narrows the scope of `Validate()` to CLI-only rules and adds a second validation layer at the public library API surface. The `Complete → Validate → Run` command pattern described here remains in force for cobra commands. The three-tier testing strategy in §4 (Unit, Integration, E2E) is superseded by [ADR-0014](0014-integration-test-tiers.md), which formalises four tiers: Unit, Library Integration (`library` tag), CLI Integration (`integration` tag), and On-Cluster Integration (`cluster_integration` tag). "Integration Tests" in §4 maps to the CLI Integration tier; "E2E Tests (Future)" maps to On-Cluster Integration.

Amended 2026-07-31: [ADR-0020](0020-repository-package-structure.md)
supersedes the package-placement guidance in §3 and Implementation Details. The
historical paths below remain unchanged.

## Context

The initial implementation of UDS CLI Next tightly coupled cobra command parsing with command execution logic. This made unit testing cumbersome, required the cobra test harness for all tests, and didn't provide a clear separation between command wiring and business logic. As the CLI grows in complexity, this approach would not scale well.

During code review of PR #9, feedback was provided (discussions [r2754876583](https://github.com/defenseunicorns/uds-cli-next/pull/9#discussion_r2754876583) and [r2754914716](https://github.com/defenseunicorns/uds-cli-next/pull/9#discussion_r2754914716)) recommending we adopt patterns from kubectl that have proven effective at scale.

## Decision

We will adopt the following architectural patterns from kubectl:

### 1. Options Struct Pattern

Every command follows the Options pattern with three methods:
- `Complete()` - Parses command-line arguments into the Options struct
- `Validate()` - Validates the Options struct
- `Run()` - Executes the command logic

This separation enables:
- Easy unit testing without cobra
- Clear separation of concerns
- Consistent command structure

### 2. IOStreams Abstraction

All commands accept `iostreams.IOStreams` for I/O, enabling:
- Easy output capture in tests
- Consistent I/O handling
- Testable commands without mocking

### 3. Package Separation

Clear separation between:
- `pkg/cmd/` - Command wiring (cobra commands)
- `pkg/bundle/`, `pkg/version/`, etc. - Business logic (no cobra)
- `pkg/iostreams/` - I/O abstraction for testing

### 4. Testing Strategy

Multi-layered testing approach:
- **Unit Tests** - Test `Complete()`, `Validate()`, and `Run()` methods independently, located alongside code in `pkg/`
- **Integration Tests** - Test full command execution with `//go:build integration` tag, located in `tests/integration/` directory
- **E2E Tests** - (Future) Full CLI workflows

## Consequences

### Positive

- **Better Testability**: Unit tests can directly test command logic without cobra
- **Clear Separation**: Business logic is separate from command wiring
- **Consistency**: All commands follow the same pattern
- **Scalability**: Pattern scales well as CLI grows
- **Easier Debugging**: Clear boundaries between components
- **Better Error Handling**: Consistent error handling via `util.CheckErr`

### Negative

- **More Boilerplate**: Each command requires Options struct and three methods
- **Learning Curve**: New contributors need to understand the pattern
- **Migration Effort**: Existing commands need to be refactored

### Neutral

- **File Organization**: More files but clearer organization
- **Test Files**: Separate unit and integration test files

## Implementation Details

### Directory Structure

```
cmd/uds-cli-next/     # Main entry point
pkg/
  cmd/                # Command wiring (cobra commands)
    cmd.go            # Root command
    bundle/           # Bundle subcommands
    version/          # Version command
    util/             # Command utilities (CheckErr, etc.)
  bundle/             # Bundle business logic (no cobra)
  version/            # Version information (no cobra)
  iostreams/          # I/O abstraction for testing
tests/                # Test files and data
  integration/        # Integration tests (mirrors pkg/ structure)
  test_data/          # Test fixtures and data files
```

### Example Command Pattern

```go
type CreateOptions struct {
    BundleFile string
    iostreams.IOStreams
}

func (o *CreateOptions) Complete(cmd *cobra.Command, args []string) error {
    if len(args) > 0 {
        o.BundleFile = args[0]
    }
    return nil
}

func (o *CreateOptions) Validate() error {
    if o.BundleFile == "" {
        return fmt.Errorf("bundle file is required")
    }
    return nil
}

func (o *CreateOptions) Run() error {
    fmt.Fprintf(o.Out, "Creating bundle from file: %s\n", o.BundleFile)
    return bundle.Create(o.BundleFile)
}
```

## References

- kubectl L&L video: https://drive.google.com/file/d/13hYXJuw28efLtFuUOXexteArEakuZdKG/view
- kubectl command patterns: `.agents/skills/example-repositories/example-repos/kubectl/pkg/cmd/`
- zarf command patterns: `.agents/skills/example-repositories/example-repos/zarf/src/cmd/`
- PR #9 discussions:
  - https://github.com/defenseunicorns/uds-cli-next/pull/9#discussion_r2754876583
  - https://github.com/defenseunicorns/uds-cli-next/pull/9#discussion_r2754914716
- Testing guide: `.agents/skills/testing/references/TESTING.md`
