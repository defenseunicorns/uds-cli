# 17. HCL `file()` Function Materialization

Date: 2026-07-28

## Status

Accepted

## Context

Bundle authors need to use externally managed text in bundle definitions,
defaults, and deploy-time config. Bundle artifacts must remain self-contained:
they cannot depend on the source machine's filesystem after they are created.

## Decision

`file(path)` reads a regular UTF-8 file and returns its contents as a string.
It is available in file-backed `bundle.uds.hcl`, `defaults.uds.hcl`, and
`config.uds.hcl`. Relative paths are resolved from the containing HCL file.
Calls may appear in locals, templates, and nested expressions.

For local source operations, including direct deploy, the function is evaluated
dynamically when the HCL is parsed. During `uds bundle create`, every `file()`
call in bundle and defaults HCL is evaluated and replaced with a string
literal before those existing HCL layers are stored in the OCI definition
manifest. `uds bundle reconfigure` materializes its replacement defaults file
the same way. As a result pulled and extracted artifacts never need the original
referenced files.

`config.uds.hcl` contains deploy-time configuration and is not embedded in the
artifact, so its `file()` are dynamically resolved.

## Alternatives Considered

### Config-only `file()`

Rejected. It prevents bundle authors from using file-backed values in bundle
metadata and defaults, even though create can snapshot those values safely.

### Include referenced files as OCI layers

Rejected. General file layers require source-path mapping, extraction and
resolution behavior, collision rules, and new manifest conventions. Replacing
calls in the already-stored HCL provides the same self-contained result using
the existing definition layers.

### Store `file()` calls unchanged in bundle artifacts

Rejected. Deploying a pulled artifact would depend on paths from its creation
environment and would not be reproducible.

## Consequences

- Created and reconfigured artifacts are portable snapshots of bundle/defaults
  file content.
- Direct source deployment continues to read current local file content.
- Inspecting stored artifact HCL shows materialized values rather than the
  original `file()` expression.
