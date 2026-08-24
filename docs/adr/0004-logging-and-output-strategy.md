# 4. Logging and Output Strategy

Date: 2026-02-24
Updated: 2026-03-30

## Changelog

| Date | Change |
|------|--------|
| 2026-02-24 | Initial ADR - established `slog` as logger, stdout/stderr separation, `--log-level` flag |
| 2026-03-30 | Adopted `console-slog` as log handler. Added `ResourcePrinter` interface for structured command output (`--output` flag with text/json/yaml). Defined result-object pattern for commands |
| 2026-06-02 | Stderr logging mechanism amended by [ADR 0012](./0012-library-logging-via-iostreams.md): the library logs through `IOStreams` instead of a process-global `slog.SetDefault()`. Stdout/output-formatting decisions below remain in force. |

## Status

Accepted. The stderr-logging mechanism in Decision §1–§4 below is superseded by [ADR 0012](./0012-library-logging-via-iostreams.md) (the library logs through `IOStreams`, not a process-global logger); the stdout/output-formatting decisions remain current.

Amended 2026-07-31: [ADR-0020](0020-repository-package-structure.md)
supersedes the command and printer package-placement references. The historical
paths below remain unchanged.

## Context

As the CLI grows, there are two distinct categories of content written to the terminal:

1. **User-facing command output** - the structured result of a command (e.g. bundle inspect printing bundle metadata). This is what the user asked for.
2. **Log messages** - diagnostic and operational information about what the CLI is doing internally (e.g. "parsing bundle file", "validation passed"). This is supplementary information.

Without a clear separation, these categories get conflated. The result is inconsistent output that is hard to test and hard for users to reason about (e.g. not knowing whether to pipe stdout or stderr, or whether a line is a result or a status message).

UDS CLI vendors Zarf as an in-process library, and both tools emit logs to stderr during operations like `uds bundle deploy`. To provide a consistent user experience, the logging approach must be unified - a single handler, configured once at startup, shared by UDS CLI code and all vendored libraries via `slog.SetDefault()`. This becomes increasingly important as more tools are vendored.

## Decision

### 1. Stdout carries structured command output only; stderr carries logs

**Stdout** is written to via `IOStreams.Out` using `fmt.Fprintf`. It is reserved exclusively for the structured result of a command - the object the user asked for. Scripts and pipelines can reliably consume stdout (e.g. `uds bundle inspect ./bundle.tar.zst -o json | jq .`). This keeps output testable - tests use `NewTestIOStreams()` which captures `Out` in a `bytes.Buffer`.

**Stderr** is written to via the `console-slog` global logger, which unifies UDS CLI Next with Zarf. All diagnostic log messages (debug, info, warn, error) go here. The global logger is available everywhere.

### 2. Log handler: `console-slog` (unified with Zarf)

The log handler for stderr is `console-slog` - the same handler Zarf uses (already vendored). It provides human-friendly colored output and unifies log appearance between UDS CLI and all vendored libraries via `slog.SetDefault()`.

### 3. `log/slog` as the logging foundation

All logging goes through `log/slog` (Go 1.21+). `console-slog` is not a separate logging framework - it is a drop-in `slog.Handler` implementation that controls how `slog` formats output. All call sites use standard `slog` APIs (`slog.Info()`, `slog.Debug()`, etc.); `console-slog` only determines what the output looks like on stderr. This means no code is coupled to `console-slog` - swapping the handler later requires changing one line in `logger.New()`, not any call sites. See [Library Comparison](#library-comparison) for why `slog` was chosen over `zap`, `klog`, and `logr`.

### 4. Log level is controlled via `--log-level` flag

The root command exposes a `--log-level` flag (debug, info, warn, error). The global logger is initialized with the parsed level in `PersistentPreRunE`. The default level is `info`. Debug statements are only visible when a user explicitly passes `--log-level debug`.

### 5. Command result objects with structured output formatting

Every command produces a result object that represents the command's output. Result objects are constructed in the command layer (`pkg/cmd/`) from return values of business logic functions. These result objects carry `json`, `yaml`, and `text` struct tags so they can be serialized in any supported format. The `text` tag provides the field label for the `TextPrinter` (human-readable output).

```go
type CreateResult struct {
    BundleName string `json:"bundleName" yaml:"bundleName" text:"Bundle Name"`
    OutputPath string `json:"outputPath" yaml:"outputPath" text:"Output Path"`
}
```


### 6. ResourcePrinter for stdout formatting

A `ResourcePrinter` interface (inspired by kubectl's printer pattern) handles formatting result objects for stdout. Three implementations - `TextPrinter` (reflection-based, reads `text` struct tags), `JSONPrinter`, `YAMLPrinter` - are selected via a simple factory. The printer is invoked in `pkg/cmd/` by the top-level command `Run()` methods - business logic in `pkg/bundle/` returns result objects but never prints them directly.

### 7. `--output` / `-o` flag for format selection

A persistent `--output` / `-o` flag on the bundle subcommand controls the stdout format. Default is `text` (human-readable). Users opt into `json` or `yaml` explicitly.

```
uds bundle inspect ./bundle.tar.zst                      # text (default, human-readable)
uds bundle inspect ./bundle.tar.zst -o json              # JSON on stdout
uds bundle inspect ./bundle.tar.zst -o yaml              # YAML on stdout
uds bundle inspect ./bundle.tar.zst -o json 2>/dev/null  # pure JSON, no logs
```

## Consequences

### Positive

- **Structured output**: `uds bundle inspect ./bundle.tar.zst -o json | jq .` works correctly for command pipelines
- **Clean stream separation**: stdout (`IOStreams.Out` via `fmt.Fprintf`) is always the result object; stderr (`console-slog`) is always diagnostic logs. Scripts can `2>/dev/null` to get pure structured output
- **Unified logging with Zarf**: UDS CLI and vendored Zarf share the same `console-slog` handler on stderr
- **Testability**: Command output captured via `NewTestIOStreams()`. Printers are independently testable. No `*slog.Logger` passing needed
- **Consistent pattern**: All commands follow the same model - business logic returns a result object, `pkg/cmd/` prints it via `ResourcePrinter`
- **Extensible**: New output formats (e.g. `table`, `wide`) can be added by implementing `ResourcePrinter` without touching command or business logic

### Negative

- **Two output mechanisms**: Contributors must know which to use - result objects via `ResourcePrinter` to stdout, diagnostics via `slog` to stderr
- **Result type maintenance**: Every command needs a result struct with `json`/`yaml`/`text` tags
- **Reflection cost**: `TextPrinter` uses reflection to walk struct fields at print time. This is negligible for a CLI (single print per command invocation) but means text output format is implicit in struct tags rather than explicit in code

### Rejected Alternatives

**TTY-aware auto-switching** - Rejected. Users should explicitly choose their output format via `--output` rather than having the CLI silently change behavior based on terminal detection.

**Alternative `slog.Handler` implementations** (e.g. `tint`, `devslog`) - Rejected due to Zarf incompatibility. Zarf uses `console-slog`, so adopting a different handler would cause log output to look different when running Zarf standalone vs through UDS CLI. Unifying on `console-slog` avoids this divergence with zero additional dependencies.

## Appendix

### Architecture

#### Stream Separation

```
┌──────────────────────────────────────────────────────────┐
│                     UDS CLI Command                      │
│                                                          │
│  ┌─────────────────────┐    ┌─────────────────────────┐  │
│  │  Business Logic     │    │  slog.* calls           │  │
│  │  returns result obj │    │  (debug, info, warn)    │  │
│  └──────────┬──────────┘    └──────────┬──────────────┘  │
│             │                          │                 │
│             ▼                          ▼                 │
│  ┌─────────────────────┐    ┌─────────────────────────┐  │
│  │  ResourcePrinter    │    │  console-slog handler   │  │
│  │  (text/json/yaml)   │    │  (colorized, leveled)   │  │
│  └──────────┬──────────┘    └──────────┬──────────────┘  │
│             │                          │                 │
│             ▼                          ▼                 │
│         STDOUT                     STDERR                │
│    (structured output)        (diagnostic logs)          │
└──────────────────────────────────────────────────────────┘
```

#### Initialization Flow

```
┌─────────────────────────────────────────────────────────────┐
│                    PersistentPreRunE                        │
│                                                             │
│  ┌──────────────┐                                           │
│  │ --log-level  │                                           │
│  │ (debug/info/ │                                           │
│  │  warn/error) │                                           │
│  └──────┬───────┘                                           │
│         │                                                   │
│         ▼                                                   │
│  ┌──────────────────┐                                       │
│  │ slog.SetDefault  │                                       │
│  │ (console-slog    │                                       │
│  │  → stderr)       │                                       │
│  └─────┬────────────┘                                       │
│        │                                                    │
│        ▼                                                    │
│   All slog.* calls                                          │
│   (UDS + Zarf)                                              │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                    Command Complete()                       │
│                                                             │
│  ┌────────────────┐                                         │
│  │ --output / -o  │                                         │
│  │ (text/json/    │                                         │
│  │  yaml)         │                                         │
│  │ default: text  │                                         │
│  └──────┬─────────┘                                         │
│         │                                                   │
│         ▼                                                   │
│  ┌───────────────────┐                                      │
│  │ ResolvePrinter    │                                      │
│  │ (text/json/yaml)  │                                      │
│  │  → stdout)        │                                      │
│  └──────┬────────────┘                                      │
│         │                                                   │
│         ▼                                                   │
│    Command Run()                                            │
│    uses printer                                             │
└─────────────────────────────────────────────────────────────┘
```

### Library Comparison

Note: `zap`, `klog`, and `logr` are all vendored transitively through other dependencies in this project, so this is a comparison of what to use as the primary logger, not whether to add a dependency.

| Library | Designed For | Dependencies | Verdict |
|---|---|---|---|
| `log/slog` | General purpose - services, CLIs, libraries | None (stdlib, Go 1.21+) | **Chosen.** Idiomatic for new Go projects, pluggable handlers, zero dependency cost. |
| `zap` | High-throughput services (Uber, Stripe, Netflix) | `go.uber.org/atomic`, `go.uber.org/multierr` | Overkill for a CLI. 2-3x faster than slog but the gap is immeasurable in a CLI context where I/O and user time dominate. |
| `klog` | Kubernetes ecosystem only | `github.com/go-logr/logr` | Not a general-purpose library. Carries k8s-specific conventions (`--v` verbosity flags) and is being actively deprecated in favor of routing through logr/slog. |
| `logr` | An interface for libraries, not applications | None | Not an implementation - it defines a logging API so libraries can avoid coupling to a concrete logger. Useful in the k8s operator ecosystem; adds indirection without benefit for a CLI application. |

`slog` is the natural fit: zero dependencies, stdlib, rich handler ecosystem (including drop-in prettification handlers like `tint` or `console-slog`), and the direction the Go community is converging on for new projects as of 2025-2026.

## References

- [ADR 0002: CLI Architecture Patterns (IOStreams abstraction)](./0002-cli-architecture-patterns.md)
- [Zarf logger](https://github.com/zarf-dev/zarf/blob/main/src/pkg/logger/logger.go)
- [console-slog](https://github.com/phsym/console-slog) - human-friendly slog handler used by Zarf
- [kubectl printer system](https://github.com/kubernetes/kubectl) - `ResourcePrinter` interface, `PrintFlags` composition
- `log/slog` docs: https://pkg.go.dev/log/slog
