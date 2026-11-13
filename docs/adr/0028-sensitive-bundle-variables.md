# 28. Sensitive bundle variables

Date: 2026-11-11

## Status

Accepted

## Context

Bundle variables can contain credentials and other values that should not appear in CLI-managed output or errors. The CLI needs sensitivity metadata without changing the values passed to package values templates or Zarf.

## Decision

Use `sensitive(value)` on values in the `variables` object in `config.uds.hcl` or `defaults.uds.hcl`. A marked nested value marks its whole top-level variable. The mark remains when defaults, config values, and `--set` overrides are merged. Values keep their existing template and Zarf behavior. The CLI omits marked values from deployment diagnostics and errors when it controls those outputs.

## Consequences

This does not guarantee that all output hides marked values. Helm writes some coalescing warnings through Go's process-global logger; those warnings can expose marked values during a values conflict. Package actions, caller hooks, and other external code also control their own output. `sensitive()` is metadata, not encryption. Materialized `sensitive(file(...))` defaults retain the function call, while the referenced file contents are stored as plaintext.

## Alternatives considered

### Keep sensitivity in a separate user-facing list

```hcl
variables = { registry = { password = "secret" } }
sensitive_variables = ["registry.password"]
```

Rejected. This keeps variable values as plain data and avoids per-value wrapper handling in Go, but the parallel path list duplicates variable paths and can drift from the values object. The implementation still carries separate sensitivity metadata internally, but authors mark values at their source.

### Keep marked wrappers through every runtime layer

```hcl
variables = {
  password      = sensitive("secret")
  registry_cert = sensitive(file("secret.txt"))
}
```

Selected as the HCL authoring syntax because it marks values where they are defined. It extends the function-registration mechanism used by `file()` and uses HCL marks to carry sensitivity. Keeping wrappers in runtime Go values would require changes across merging, package values templates, and Zarf. Instead, values keep their existing Go forms and sensitivity travels separately at the top-level variable boundary.

### Track secret strings and replace exact matches in output

```go
output = strings.ReplaceAll(output, "secret", "****")
```

Rejected. Exact matches miss transformed, encoded, or partial values; errors can expose values through wrapped causes; and replacement cannot protect output outside the configured logger boundary.

## Implementation details

Keeping `Variables` as plain data preserves public Go map literals and indexing, and compatibility with Go templates, `Flatten`, and Zarf. Sensitivity metadata travels alongside values and through merges, avoiding wrappers and unwrapping across runtime layers. Wrapping `Variables` alone cannot prevent leaks after plain values reach the backend, so safe output boundaries are still required.
