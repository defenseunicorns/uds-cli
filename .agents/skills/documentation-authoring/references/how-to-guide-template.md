# How-To Guide Template

How-to guides walk a reader through one task. Keep the guide focused on a single outcome and verify every command against the active UDS CLI implementation.

How-to guides normally live under `docs/how-to-guides/<section>/` and use the `.mdx` extension.

## Template

````mdx
---
title: Topic name
description: Configure, enable, or use one UDS CLI workflow and state the result the reader will achieve.
sidebar:
  order: X.XXX
---

import { Steps } from '@astrojs/starlight/components';
{/* Add Tabs and TabItem only when the guide has supported mode or platform choices. */}

## What you'll accomplish

Briefly describe the task and the expected result.

## Prerequisites

- [UDS CLI](https://github.com/defenseunicorns/uds-cli/releases) installed
- Access to any required registry, OCI artifact, filesystem path, or Kubernetes cluster
- Required credentials, trust policy, or feature flag

## Before you begin

<!-- Optional. Include only context needed to understand the procedure. -->

State relevant defaults, mode selection, or repository layout.

## Steps

<Steps>

1. **Create the required files**

   Explain the starting state and show complete, copy-pasteable examples.

   ```hcl title="bundle.uds.hcl"
   // Minimal example verified against the current implementation.
   ```

2. **Run the workflow**

   Include the exact command, required flags, expected artifact path, and any required feature flag.

   ```bash
   CLI_FEATURES=NextMode=true uds bundle create ./bundle
   ```

3. **Verify the result**

   Show the command or observable output that proves the task succeeded.

</Steps>

## Verification

Confirm the expected artifact, output, or deployed state:

```bash
# Use the actual command and output shape for this workflow.
```

## Troubleshooting

### Problem: Short description of the issue

**Symptom:** What the reader sees.

**Solution:** The smallest actionable fix, including the command or file change.

## Related documentation

- [Reference page](/cli/reference/section/page/) - exact commands, flags, or file fields
- [Concepts page](/cli/concepts/section/page/) - background needed to understand the workflow
- [External documentation](https://example.com) - relevant upstream behavior
````

## Conventions

### Structure

- Use the section order: What you'll accomplish, Prerequisites, optional Before you begin, Steps, Verification, optional Troubleshooting, Related documentation.
- Use one primary task per guide. Split unrelated workflows into separate pages.
- Use `<Steps>` for procedures. Use `<Tabs>` and `<TabItem>` when the reader must choose between supported modes or platforms.
- Do not repeat the frontmatter `title` as a body heading. Starlight renders the page title.
- End with `## Related documentation` and use a flat list with a short description for every link.

### UDS CLI workflows

- Keep Legacy and Next instructions visibly separate. Do not imply that one command or file format works in both modes without verification.
- For Next examples, include `CLI_FEATURES=NextMode=true` or `--features=NextMode=true` on every command that needs Next mode.
- Use `bundle.uds.hcl` for the bundle definition, `defaults.uds.hcl` for build-time defaults, and `config.uds.hcl` for deploy-time configuration. Explain which file each example changes.
- Include required source artifacts, directories, variables, credentials, and trust-policy inputs. Mark placeholders explicitly.
- Verify command names, positional arguments, flags, defaults, output paths, and failure behavior against Cobra help and tests.
- Do not claim that package signature verification proves bundle integrity. Explain the separate bundle signature check when relevant.
- Mark unsigned or skipped-verification workflows as local alpha workflows and state the security consequence.

### Frontmatter and formatting

- `description:` is required and should start with an active verb such as `Configure`, `Enable`, or `Create`.
- Use sentence case for titles and headings. Do not quote plain YAML title values.
- Use `.mdx` for guides and import only the Starlight components used by the page.
- Use titled code blocks when the filename matters, for example `` ```hcl title="bundle.uds.hcl" ``.
- Do not use horizontal rules as section dividers.
- Use internal links with the `/cli/` route when linking between pages in the published UDS Docs site.
