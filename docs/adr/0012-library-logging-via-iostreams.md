# 12. Library Logging via IOStreams

Date: 2026-06-02

## Status

Accepted. Amends the stderr-logging mechanism of [ADR-0004](0004-logging-and-output-strategy.md); the stdout/output-formatting decisions there remain in force.

## Context

The library logged through the process-global `slog.Default()`. Consumers (e.g. the Remote Agent) could not control where a given call's diagnostics went without monkey-patching `slog.SetDefault` under a mutex, and concurrent operations interleaved their output. First-class library support requires that diagnostics be controllable and isolated per call.

`IOStreams` already carries the consumer's `In`/`Out`/`ErrOut` and is threaded onto every operation's options struct, so it is the natural owner of all library output - results and diagnostics alike.

## Decision

1. **`IOStreams` is the single sink for all library output.** Structured results go to `Out`; diagnostics go to `ErrOut`. A consumer controls all output solely by providing an `IOStreams`.
2. **`IOStreams` carries leveled logging.** It exposes `Debug`/`Info`/`Warn`/`Error` that write to `ErrOut`, and library code logs through them. No library path reads or writes `slog.Default()`.
3. **The log level comes from config**, bound onto `IOStreams` once at each public library entrypoint:

   ```go
   streams = logger.Bind(opts.Streams, opts.Config.Global.LogLevel)
   streams.Info("deploying bundle", "packages", len(b.Packages))
   ```

   Stateful components (`HCLParser`, `ZarfDeployer`, `ZarfRemover`) hold the bound `IOStreams`; free functions receive it as a parameter.

Rejected: carrying the logger in `context.Context`. It is implicit - the dependency is invisible in signatures, and a caller must remember to seed the context or silently get no output. `context.Context` is reserved for cancellation.

## Consequences

### Positive

- **Consumer-controlled, isolated output**: each call logs to the `IOStreams` it was given; concurrent operations with separate streams never interleave.
- **No global state**: the library never mutates `slog.Default()`, so embedding it is safe.
- **Explicit**: logging is visible at every call site through the `IOStreams` parameter or receiver field.
- **Aligns with output synchronization**: because results and logs share one writer, synchronizing at the `IOStreams` level covers both.

### Negative

- **Plumbing**: functions that log take an `IOStreams` parameter (or hold one as a field). This is the cost of explicitness over an ambient global.

### Neutral

- The `console-slog` handler and `log/slog` foundation from ADR-0004 are unchanged; `IOStreams.Debug`/`Info`/`Warn`/`Error` delegate to a `*slog.Logger` built from `ErrOut` and the configured level.

## References

- [ADR-0004 - Logging and Output Strategy](0004-logging-and-output-strategy.md)
- [ADR-0002 - CLI Architecture Patterns (IOStreams abstraction)](0002-cli-architecture-patterns.md)
- kubectl `genericclioptions.IOStreams` - https://pkg.go.dev/k8s.io/cli-runtime/pkg/genericclioptions#IOStreams
- `log/slog` - https://pkg.go.dev/log/slog
