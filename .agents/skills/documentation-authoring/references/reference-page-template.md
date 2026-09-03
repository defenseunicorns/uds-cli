# Reference Page Template

Reference pages document stable UDS CLI behavior that readers look up: commands, flags, file fields, defaults, output, and failure behavior. They are not task-oriented tutorials.

Reference pages normally live under `docs/reference/<section>/` and use the `.mdx` extension unless the surrounding section uses another format.

## Template

````mdx
---
title: Topic name
description: Reference the UDS CLI commands, fields, or artifacts covered by this page and identify the intended reader.
sidebar:
  order: X.XXX
---

One or two sentences defining the exact behavior covered by this page.

## Command or file surface

Briefly state what this surface controls.

| Field | Type | Default | Description |
|---|---|---|---|
| `field.name` | string | `unset` | What the field controls |
| `field.other` | boolean | `false` | What the field controls |

> [!NOTE]
> Include only non-obvious timing, mode, ordering, compatibility, or security behavior.

```bash
# Show the exact command form and relevant output.
CLI_FEATURES=NextMode=true uds bundle create ./bundle --help
```

## Legacy and Next behavior

Describe differences only when both implementations support the surface. Link to the relevant mode-specific page when the workflows diverge.

### Legacy

Document the verified command, input format, defaults, and output.

### Next

Document the verified command, input format, defaults, and output. Include the required feature flag in examples.

## Failure behavior

| Condition | Result | Recovery |
|---|---|---|
| Missing required input | Exact error or failure behavior | Corrective action |

## Related documentation

- [How-to guide](/cli/how-to-guides/section/page/) - task-oriented usage
- [Concepts page](/cli/concepts/section/page/) - background explanation
- [Source or upstream documentation](https://example.com) - authoritative implementation detail
````

## Conventions

### Scope

- Document stable, lookup-oriented behavior: exact commands, flags, file fields, defaults, artifact names, output, and failure behavior.
- Do not add sections such as What you'll accomplish, Prerequisites, Steps, or Verification. Those belong in how-to guides unless a short verification example is necessary to define command output.
- Keep one command, file, or artifact surface as the primary subject. Link to related surfaces rather than duplicating their reference.

### Tables and examples

- Use tables for fields, flags, defaults, and failure conditions.
- Wrap command names, flags, paths, field names, and literal values in backticks.
- Use `unset` for fields with no default or required fields.
- List enum values explicitly, for example `` `legacy` | `next` ``.
- Show only the relevant file or command context. Do not invent omitted defaults or boilerplate.
- Include the Next feature flag in every Next mode command example.
- Verify file fields against the parser, schema, or source of truth. Verify flags and output against Cobra help, implementation, and tests.

### Frontmatter and links

- `description:` is required and should state the behavior covered and its audience.
- Use sentence case and three-decimal sidebar ordering.
- Do not repeat the frontmatter `title` as a body heading.
- End with `## Related documentation`, using a flat list and a short description for every link.
- Use internal links with the `/cli/` route when linking between published UDS Docs pages.

### Security and compatibility

- Describe trust-policy inputs, credentials, and verification boundaries explicitly.
- Do not claim package signature verification proves bundle integrity. Bundle signature verification is a separate artifact check.
- State when behavior is Legacy-only, Next-only, alpha, or dependent on a feature flag.
- Do not document future support or inferred behavior as current behavior.
