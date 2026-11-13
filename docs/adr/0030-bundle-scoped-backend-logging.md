# 30. Bundle-scoped backend logging

Date: 2026-11-13

## Status

Accepted. Supersedes the package-level logging lifetime in [ADR-0029](0029-sensitive-backend-logging.md) and adds a deployment-scoped exception to [ADR-0012](0012-library-logging-via-iostreams.md)'s prohibition on changing global logging. Cancellation error handling from ADR-0029 remains unchanged.

## Context

Helm bypasses its supplied logger in some paths. Values-conflict warnings use the global standard logger, and other diagnostics use the global `slog` logger. Per-package suppression requires shared lifetime counting when package deployments overlap. IOStreams cannot intercept these global calls by itself.

## Decision

Own and silence the global `log` and `slog` loggers once per bundle deployment. Restore their previous configuration when that operation returns. Independent bundle deployments in one process wait for this scope; packages within the active bundle retain their configured concurrency. Separate CLI processes remain independent. Helm and Zarf dependencies remain unchanged.

## Implementation details

The bundle is the smallest existing operation that owns all parallel package deployments. There is no multi-bundle deployment command. Acquiring one lock before bundle execution and restoring logging after it finishes removes the per-package counter and writer ownership bookkeeping. The orchestrator waits for all in-flight package calls before returning, including on errors and cancellation, so logging is not restored while a sibling package is still deploying.

Use a one-permit semaphore so waiting callers can cancel. Save the default structured logger plus the standard logger’s writer and flags. Set the default structured logger to discard; Go also redirects the standard logger through that handler. Defer restoration and release together. Keep UDS progress and per-operation Zarf logging on explicit IOStreams loggers.

Every bundle takes this scope, even when its initial configuration is unmarked: package hooks can replace configuration during execution. A private context marker rejects deployments invoked from a deployment callback with the supplied context, avoiding a wait on the callback’s own bundle. Callbacks must propagate that context and must not wait for another deployment started with an unrelated context.

Running Helm in a subprocess would isolate its global loggers and allow independent bundles to overlap without affecting the host’s logging. We reject that alternative because it requires substantial inter-process coordination: transporting package layouts, options and original values, returning structured results and errors, coordinating cancellation and cleanup, and integrating Go callbacks and custom loaders that cannot cross a process boundary. That machinery is disproportionate to the logging requirement.

## Consequences

Package parallelism is preserved, but concurrent bundle operations within one Go process serialize. Global diagnostics from unrelated code are also muted during the bundle scope, including during unmarked deployments. Applications must not reconfigure the global loggers while deployment is running. Explicit IOStreams output remains available; package actions and other code writing directly to stdout or stderr remain outside this protection.
