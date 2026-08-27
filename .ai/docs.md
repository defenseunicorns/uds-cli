# Documentation Development

## Scope

These instructions apply to documentation under `docs/`. UDS CLI documentation is published inside the UDS Docs site under the `/cli/` route. Use the repository's existing `/cli/...` links for cross-page references.

## Audience

- Getting-started pages target platform engineers running their first local or development workflow.
- How-to guides target bundle authors, release engineers, registry operators, and cluster operators. State the starting condition and the expected result.
- Reference pages target users who need exact command, flag, file-format, or artifact behavior.

## Structure

- Use sentence case for titles and headings.
- Use three decimal places for sidebar order values, such as `3.500`.
- Add a concise `description` that states the page outcome.
- Use `## What you'll accomplish`, `## Prerequisites`, `## Steps`, `## Verification`, and `## Related documentation` for task-oriented guides. Add `## Troubleshooting` when the workflow has likely failure modes.
- Import `Steps` from `@astrojs/starlight/components` when the page uses numbered procedure steps.
- End related-documentation links with a short description of what each linked page provides.
- Keep one primary task per how-to guide. Link to another guide instead of adding an unrelated workflow.

## Accuracy

- Treat the active Go command implementation, Cobra help, tests, and current CLI output as the source of truth.
- Before documenting a command, verify its `Use` form, positional arguments, flags, defaults, output paths, and failure behavior under `internal/cli/bundle/` and `internal/bundle/`.
- Use `CLI_FEATURES=NextMode=true` or `--features=NextMode=true` in every Next mode command example.
- Keep `bundle.uds.hcl`, `defaults.uds.hcl`, and `config.uds.hcl` responsibilities distinct. Do not describe deploy-time configuration as build-time defaults.
- Do not claim that package signature verification proves bundle integrity. Bundle signature verification is a separate artifact check.
- Mark unsigned or skipped-verification workflows as local alpha workflows and explain the security consequence.
- Every copy-paste example must include its required files, directories, variables, credentials, and trust-policy inputs, or explicitly identify placeholders.
- Do not invent command flags, output formats, artifact names, registry behavior, or future support. Confirm them in source or tests.

## Language

- Use direct, active, imperative prose. Prefer "Run the command" to "The command should be run."
- Address the reader as "you" when instructions need an actor.
- Keep sentences concrete and short enough to scan.
- Explain why a non-obvious command or flag is needed.
- Avoid filler, marketing language, vague claims, fake quotations, repeated conclusions, and generic headings such as "Additional Information."
- Do not use em dashes. Use a period, colon, comma, or parentheses instead.
- Do not use horizontal-rule dividers as prose decoration.
- Use fenced code block titles when a filename or format matters. Add comments only for non-obvious fields or intentional placeholders.

## Review Severity

Classify documentation findings as:

- P1: Incorrect or unsafe instructions that can cause a failed deployment, invalid artifact, or security mistake.
- P2: Materially incomplete or misleading workflow guidance that blocks a common user path.
- P3: Significant structure, audience, voice, or consistency problem.
- P4: Cosmetic wording, formatting, or navigation issue.

Fix P1 and P2 findings before delivery. Fix P3 and P4 findings when they are local and unambiguous.

## Validation

- Run `git diff --check`.
- Run the smallest relevant unit tests for source-backed behavior.
- Run the documentation build from the docs integration when documentation changes affect routing or MDX.
- Check every new internal link and every command example against the active implementation.
- Review the final diff for unsupported claims, inconsistent headings, missing prerequisites, and absent related-link descriptions.
