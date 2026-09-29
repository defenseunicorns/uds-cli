# Release Notes Template

Release notes document what changed in one UDS CLI release, why it matters, and what action an existing user may need to take. They are not a replacement for standard setup or workflow documentation.

Release notes normally live under `docs/operations/release-notes/` and use the `.mdx` extension.

## File naming

Use the release version with dots replaced by hyphens, for example `0-70.mdx` or `1-0.mdx`.

## Template

````mdx
---
title: UDS CLI X.Y
description: UDS CLI X.Y release notes covering the most significant breaking change, feature, or behavior update.
sidebar:
  order: X.XXX
---

> [!NOTE]
> Follow the current [getting started guidance](/cli/getting-started/overview/) for standard installation and workflow setup.

Summary of the release and its impact on CLI users in two or three sentences.

### Breaking changes

| Change | Impact | Action required |
|---|---|---|
| Description | What changes for existing users | Required migration or configuration change |

<!-- Omit this section when there are no breaking changes. -->

### Notable features

- **Feature name:** what it does and why it matters. Link to the relevant guide when available.

### Dependency updates

| Dependency | Previous | Updated |
|---|---|---|
| Dependency name | X.Y.Z | [X.Y.Z](https://upstream-release-url) |

## Upgrade considerations

<!-- Omit this section when no version-specific action is required beyond standard guidance. -->

### Pre-upgrade steps

1. **Step description**

   Details and commands, including any required backup or compatibility check.

### Post-upgrade verification

1. **Step description**

   Details and commands that prove the version-specific change is active.

## Related documentation

- [Getting started guidance](/cli/getting-started/overview/) - standard installation and workflow setup
- [UDS CLI X.Y changelog](https://github.com/defenseunicorns/uds-cli/blob/main/CHANGELOG.md#anchor) - complete change list
- [Full diff](https://github.com/defenseunicorns/uds-cli/compare/vX.W.Z...vX.Y.Z) - changes between releases
- [Related guide](/cli/how-to-guides/section/page/) - detailed usage instructions
````

## Conventions

### Scope and structure

- Cover one version per page.
- Focus on breaking changes, notable features, behavior changes, dependency updates, known issues, and version-specific upgrade actions.
- Do not repeat standard installation, upgrade, or verification procedures. Link to the maintained guide instead.
- Omit empty sections, including Breaking changes and Upgrade considerations.
- Use `## Related documentation` as the final section with a flat list and descriptions.

### Content accuracy

- Verify release versions, dates, flags, commands, artifact names, and compatibility claims against the release metadata and implementation.
- Explain whether changes apply to Legacy, Next, or both.
- Link notable features to the relevant how-to or reference page when one exists.
- Link changelog entries to the repository changelog anchor and use a full diff from the latest patch of the previous minor release to the latest patch of the current release.
- Link dependency versions to meaningful upstream release pages when available.

### Frontmatter and formatting

- `description:` is required and should summarize the most significant release change in active prose.
- Use the title format `UDS CLI X.Y` and the repository's three-decimal sidebar ordering.
- Do not repeat the frontmatter `title` as a body heading.
- Use callouts only for upgrade-impacting notes, warnings, or breaking changes.
- Use titled code blocks when a filename matters and include the Next feature flag in Next command examples.
- Use internal links with the `/cli/` route when linking between published UDS Docs pages.
