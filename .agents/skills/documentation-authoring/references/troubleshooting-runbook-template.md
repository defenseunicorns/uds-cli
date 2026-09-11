# Troubleshooting Runbook Template

Troubleshooting runbooks help users diagnose and resolve one UDS CLI failure or operational problem. Each runbook should identify the symptom, gather evidence, apply a targeted fix, and verify recovery.

Runbooks normally live under `docs/operations/troubleshooting-and-runbooks/` and use the `.mdx` extension.

## Template

````mdx
---
title: Topic name
description: Diagnose and resolve the UDS CLI problem covered by this runbook and verify the recovered state.
sidebar:
  order: X.XXX
---

import { Steps } from '@astrojs/starlight/components';

## When to use this runbook

Use this runbook when:

- Observable symptom or error message
- Failed command, artifact state, or environment condition

**Example error:**

```plaintext
Exact error message or output the reader would see
```

## Overview

This problem is typically caused by one of the following:

1. **Cause A:** short explanation
2. **Cause B:** short explanation

## Pre-checks

<Steps>

1. **Confirm the current command and mode**

   ```bash
   uds bundle --help
   ```

   **What to look for:** the command, feature flag, and input format in use.

2. **Collect the relevant evidence**

   ```bash
   # Use the actual diagnostic command for this failure.
   ```

   **What to look for:** exact errors, missing files, invalid values, or trust failures.

</Steps>

## Procedure

### Cause A: description

<Steps>

1. **Apply the fix**

   ```bash
   # Use a copy-pasteable corrective command.
   ```

2. **Retry the affected operation**

   Include the complete command and any required feature flag or input.

</Steps>

### Cause B: description

<Steps>

1. **Apply the fix for cause B**

   Provide specific, actionable instructions.

</Steps>

## Verification

Confirm that the operation now succeeds and that the expected artifact or state exists:

```bash
# Use the actual verification command for this workflow.
```

**Success indicators:**

- Expected output or artifact
- No recurrence of the original error

## Additional help

If the runbook does not resolve the problem:

1. Collect the command, version, mode, inputs, and relevant output.
2. Check [UDS CLI GitHub Issues](https://github.com/defenseunicorns/uds-cli/issues) for known issues.
3. Open an issue with the collected details and a minimal reproduction.

## Related documentation

- [How-to guide](/cli/how-to-guides/section/page/) - supported task workflow
- [Reference page](/cli/reference/section/page/) - exact command or file behavior
- [Concepts page](/cli/concepts/section/page/) - background explanation
````

## Conventions

### Structure

- Keep the section order: When to use this runbook, Overview, Pre-checks, Procedure, Verification, Additional help, Related documentation.
- Organize the Procedure by root cause when multiple causes are common. For a single cause, use one procedure subsection.
- Use `<Steps>` for diagnostic and corrective procedures.
- Keep diagnostics copy-pasteable and explain what output matters. Do not tell the reader only to "check the logs" or "contact support".
- Put prevention tips in a relevant `> [!TIP]` callout. Do not create a separate Prevention section.

### UDS CLI behavior

- State whether the runbook applies to Legacy, Next, or both.
- Include `CLI_FEATURES=NextMode=true` or `--features=NextMode=true` in every Next command example.
- Verify commands, flags, input files, output paths, and error messages against the active implementation and tests.
- Include version, mode, trust-policy, credential, and environment details when they affect diagnosis.
- Do not recommend destructive cleanup, cluster operations, or shared-state changes without clearly stating the impact and prerequisite confirmation.

### Frontmatter and formatting

- `description:` is required and should start with an active verb such as `Diagnose`, `Recover`, or `Resolve`.
- Use sentence case and three-decimal sidebar ordering.
- Do not repeat the frontmatter `title` as a body heading.
- Use `plaintext` for error output and titled code blocks when a filename matters.
- Use internal links with the `/cli/` route when linking between published UDS Docs pages.
