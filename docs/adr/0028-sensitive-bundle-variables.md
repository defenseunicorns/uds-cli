# 28. Sensitive bundle variables

Date: 2026-11-11

## Status

Accepted. Adds a narrow exception to [ADR-0012](0012-library-logging-via-iostreams.md) for standard-log redaction during sensitive backend deployment.

## Context

Bundle variables can contain credentials and other values that should not
appear in CLI-managed output or errors. The CLI needs sensitivity metadata
without changing the values passed to package values templates or Zarf.

## Decision

Use `sensitive(value)` on values in the `variables` object in `config.uds.hcl`
or `defaults.uds.hcl`. A marked nested value marks its whole top-level
variable. The mark remains when defaults, config values, and `--set` overrides
are merged. Values keep their existing template and Zarf behavior. The CLI
omits marked values from deployment diagnostics and errors when it controls
those outputs. Redacted errors preserve standard cancellation and deadline
identities without retaining backend messages.

Helm's value-conflict warnings use Go's process-global standard logger and bypass IOStreams and the logger supplied through Zarf. Around sensitive Zarf backend deployment, replace exact known sensitive values in those logs with `***`. This is the closest available boundary without changing Helm or Zarf. Keep context-log suppression and error redaction unchanged.

## Consequences

Exact matching can miss transformed, escaped, or partial values. Matching text in unrelated standard logs is also redacted. Direct stdout/stderr writes and explicit loggers that bypass the standard logger remain outside this filter, including such output from package actions or caller hooks. Applications must not reconfigure global logging during deployment.

Concurrent deployments remain parallel. Their combined secrets stay registered until the last overlapping registered backend call finishes, then the original writer is restored. This can redact values belonging to a call that has already finished.

Sensitivity is metadata, not encryption. Materializing `sensitive(file(...))` retains the `sensitive()` wrapper but replaces `file(...)` with the plaintext file contents.

## Alternatives considered

### Keep sensitivity in a separate user-facing list

```hcl
variables = { registry = { password = "secret" } }
sensitive_variables = ["registry.password"]
```

Rejected. This keeps variable values as plain data and avoids per-value wrapper
handling in Go, but the parallel path list duplicates variable paths and can
drift from the values object. The implementation still carries separate
sensitivity metadata internally, but authors mark values at their source.

### Use per-value syntax with separate runtime metadata

```hcl
variables = {
  password      = sensitive("secret")
  registry_cert = sensitive(file("secret.txt"))
}
```

Select `sensitive(value)` as the HCL authoring syntax because it marks values where they are
defined. It extends the function-registration mechanism used by `file()` and
uses HCL marks to carry sensitivity. Keeping wrappers in runtime Go values
would require changes across merging, package values templates, and Zarf.
Instead, values keep their existing Go forms and sensitivity travels separately
at the top-level variable boundary.

### Track secret strings and replace exact matches in output

```go
output = strings.ReplaceAll(output, "secret", "***")
```

Selected only as best-effort protection for standard logs. It preserves useful warning text without depending on Helm's message formats. It cannot replace context-log suppression or error redaction because transformed values and wrapped causes can still expose secrets.

### Suppress all standard logs

Rejected because it removes useful diagnostics, including unrelated logs.

### Change Helm/Zarf or isolate Helm in a subprocess

Dependency changes are outside scope. A subprocess would require additional coordination for inputs, results, cancellation, and hooks.

### Use only an atomic counter

Rejected because registration must coordinate the saved writer, combined secrets, and global writer installation. An atomic count alone allows overlapping calls to overwrite filters or restore logging too early. A channel-based coordinator or custom atomic state machine adds complexity without improving deployment concurrency.

## Implementation details

Keeping `Variables` as plain data preserves public Go map literals and indexing,
and compatibility with Go templates, `Flatten`, and Zarf. Sensitivity metadata
travels alongside values and through merges, avoiding wrappers and unwrapping
across runtime layers. Wrapping `Variables` alone cannot prevent leaks after
plain values reach the backend, so safe output boundaries are still required.

`backendLogRedactor` owns collection and filtering behind its constructor and `Run` method. Snapshot marked values before deployment hooks can change them. Collect scalar leaves of marked top-level variables, skip empty strings to avoid matching every position, and match longer values first so a shorter secret cannot leave a longer secret's suffix exposed.

A short mutex protects registration and cleanup only. Backend deployment runs outside it. Each registration installs an immutable writer snapshot that forwards directly to the original writer. The filter never takes the registration mutex: Go's logger holds its own output lock while calling `Write`, so taking our mutex there could deadlock against registration calling `log.SetOutput`.

Keep the combined secrets until the last call finishes to avoid per-secret removal bookkeeping. Deferred cleanup restores logging on success, errors, or panics. `Write` reports the original input length because redaction can shorten the forwarded message.
