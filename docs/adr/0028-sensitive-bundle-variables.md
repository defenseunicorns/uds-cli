# 28. Sensitive bundle expressions

Date: 2026-11-11

## Status

Accepted.

## Decision

Use `sensitive(value)` in any evaluated attribute in `bundle.uds.hcl`, `config.uds.hcl`, or `defaults.uds.hcl`. Marks propagate through locals and expressions. Values keep their HCL types. Block labels and `depends_on` use static syntax.

```hcl
options {
  uds_cache = sensitive("/private/cache")
}

variables = {
  registry = { token = sensitive("example-token") }
}
```

- **Options.** Overriding a marked option with a flag such as `--uds-cache` also marks the effective value.
- **Variables.** Marking `registry.token` marks all of `registry`. Marks survive overrides from defaults to config to `--set`.
- **Deploy flags.** Both `bundle deploy` and `bundle dev deploy` accept `--set 'replicas=sensitive(3)'`. This marks the numeric value and any later override of `replicas`. Shell history and process listings can expose the argument.
- **Files.** `sensitive(file("token.txt"))` resolves the path relative to the source document. Only file-backed HCL supports `file()`. Materializing the artifact preserves the mark and stores the plaintext contents.

## Diagnostics

Nested CLI and library operations share known values from source preparation through deployment. Each operation masks them in diagnostics and returned errors with `***`. Inspect and reconfigure hide archive filenames that can contain unverified metadata. Templates, Zarf, and requested results such as `inspect -o json` receive actual values. HCL errors use the same categories and source positions for marked and unmarked expressions. Go callers can inspect `*bundle.HCLDiagnosticError` and `*bundle.MaskedError` with `errors.As`. Recovered fields can contain original values.

Helm writes some warnings to Go’s process-wide standard logger. As an exception to [ADR-0012](0012-library-logging-via-iostreams.md), concurrent backend deployments share a filter until the last registered call finishes. Exact matching can also mask unrelated text and miss partial or transformed values. Direct writes from hooks or package actions bypass the filter.

## Rationale

Source HCL, artifact HCL, and `--set` share one function table. The Go decoder records marked values for diagnostics and passes ordinary values to the schema, so new fields inherit this behavior.

```hcl
variables = {
  password      = sensitive("secret")
  registry_cert = sensitive(file("secret.txt"))
}
```

We rejected a separate path list because it can drift from the variables it describes.

```hcl
variables = { registry = { password = "secret" } }
sensitive_variables = ["registry.password"]
```

Exact replacement preserves useful warnings. UDS also limits backend logging and sanitizes returned errors.

```go
output = strings.ReplaceAll(output, "secret", "***")
```
