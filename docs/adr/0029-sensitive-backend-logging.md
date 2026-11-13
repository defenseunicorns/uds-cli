# 29. Sensitive backend logging

Date: 2026-11-13

## Status

Accepted. Supersedes the Helm output limitation in [ADR-0028: Sensitive bundle variables](0028-sensitive-bundle-variables.md#consequences).

## Context

Helm values-conflict warnings use Go’s process-global standard logger and can include sensitive values. Discarding Zarf’s context logger does not suppress those warnings. Redacting backend errors also discarded cancellation and deadline identities needed by library callers.

## Decision

During a sensitive backend deployment, temporarily set the standard logger’s output to `io.Discard`. Track overlapping sensitive deployments under a mutex; save the previous writer on entry to the first deployment and restore it after the last exits. Restore on success, error, or panic. Keep deployment concurrency and the original values passed to Zarf.

Retain `context.Canceled` and `context.DeadlineExceeded` identities when redacting deployment, values-file parsing, and template execution errors. Retain only these standard sentinels and the UDS error category, never the original backend error or its value-derived message.

## Consequences

This deliberately suppresses unrelated standard logs from the embedding application, unmarked deployments, and other concurrent callers while any sensitive backend deployment is active. Applications must not reconfigure the standard logger during that interval. UDS cannot coordinate logger changes made by external code. Context-based progress output remains available. Package actions, hooks, and other code that write through different outputs remain outside this protection.

## Alternatives considered

- Keep the previous boundary: leaves verified Helm warnings able to expose marked values.
- Serialize all backend deployments: unnecessarily removes package concurrency and still affects unrelated process logging.
- Add injectable logging upstream: avoids global effects but requires changes in Helm and Zarf; reconsider this boundary when that integration is available.
