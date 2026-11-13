# 28. Sensitive bundle variables

Date: 2026-11-11

## Status

Accepted. Adds a deployment-scoped exception to [ADR-0012](0012-library-logging-via-iostreams.md)'s prohibition on changing global logging.

## Context

Bundle variables can contain secrets. Helm can print them through global loggers that bypass IOStreams.

## Decision

### Sensitivity metadata

Support `sensitive(value)` inside config and defaults variables. Nested marks apply to the whole top-level variable and survive overrides. Store marks separately so templates, Zarf, and public Go values remain unchanged.

### Diagnostic redaction

Suppress sensitive backend and template diagnostics. Omit raw YAML error content. Preserve UDS error categories and standard cancellation/deadline identities without retaining secret-bearing causes.

### Backend logging

Silence global `log` and `slog` output for each bundle deployment. Keep explicit IOStreams progress visible. Do not patch Helm or Zarf.

## Implementation details

One cancellable lock owns global logging for the whole bundle. This removes per-package reference counting while preserving parallel package deployment. The orchestrator waits for in-flight packages before returning; deferred cleanup restores the previous logger, writer, and flags before releasing the lock.

Every bundle takes the lock because package hooks can introduce sensitive configuration. Nested deployments using the callback's context are rejected to prevent deadlock.

Running Helm in a subprocess would isolate logging, but requires substantial inter-process coordination for inputs, results, errors, cancellation, cleanup, callbacks, and custom loaders. We reject that complexity for this requirement.

## Alternatives considered

### Sensitivity metadata

- **Separate sensitivity list:** rejected because duplicated variable paths can drift from their definitions.
- **Runtime value wrappers:** rejected because they require changes across public Go values, merging, templates, and Zarf integration.

### Diagnostic redaction

- **String replacement:** rejected because it misses transformed or partial secrets and wrapped error causes.

### Backend logging

- **Per-package suppression:** rejected because shared reference counting and restoration bookkeeping add complexity without improving package parallelism.
- **Serialize packages:** rejected because it sacrifices package parallelism.
- **Permanent global suppression:** rejected because it also silences unrelated host logs outside deployment.
- **Patch Helm/Zarf:** rejected because maintaining dependency patches or forks is outside our maintenance budget.
- **Helm subprocess:** rejected because coordinating inputs, results, cancellation, cleanup, and callbacks across processes is disproportionate to this requirement.

## Consequences

- Independent bundles serialize within one process; package concurrency and separate CLI processes are unaffected.
- Unrelated global logs are muted during deployment. Applications must not reconfigure global logging. Callbacks must propagate their supplied context and must not wait for deployments started with an unrelated context.
- Sensitivity is metadata, not encryption. Artifact defaults remain plaintext. Direct output from package actions and external code remains outside this protection.
