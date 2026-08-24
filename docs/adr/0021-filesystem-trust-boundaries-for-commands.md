# 21. Filesystem trust boundaries for commands

Date: 2026-08-12

## Status

Accepted

## Context

UDS CLI Next commands work with two different kinds of filesystem input: trusted
authoring input and untrusted content.

Bundle authors use the CLI as a build and development tool. They point the CLI
at files they control: `bundle.uds.hcl`, `defaults.uds.hcl`, `config.uds.hcl`,
values files, local Zarf packages, temporary directories, and output
directories. Those inputs need to support normal repository layouts. A bundle in
a monorepo may read `../shared/values.yaml`. A config file may use `file()` to
read a certificate from an absolute path. A local package source may live
outside the bundle directory. The CLI should preserve those authoring workflows.

Untrusted content has a different threat model. A user may pass one artifact or
OCI reference to a command, but that input can hide many filesystem paths inside
archive entries, OCI layer titles, manifest annotations, values layer names, and
package layer titles. The user did not choose those paths; the content did. A
command that processes that content may create workspaces, extract archives,
materialize OCI layers, stage packages, and read generated files. For example, a
malicious archive could place a symlink or hardlink inside the workspace that
points to `/etc/passwd`, then rely on later package staging or values processing
to read that path as if it belonged to the artifact.

Without a filesystem trust boundary, commands can either block valid authoring
workflows or allow untrusted content to steer filesystem operations outside the
root selected for that operation.

## Decision

We trust paths the user or bundle author writes. We do not trust paths hidden
inside artifacts, archives, packages, OCI metadata, HCL, or values data.

When we trust the path, the CLI may follow it on the host filesystem. This keeps
authoring workflows such as `../shared/values.yaml`.

When we do not trust the path, the CLI contains the filesystem operation. The
command chooses a root directory for the work, and the untrusted path can only
affect files inside that root. It cannot use `..`, absolute paths, symlinks, or
hardlinks to make the CLI read, write, link, rename, or delete files elsewhere.

### Commands that process untrusted content

Commands in this category include:

- `uds bundle inspect <bundle.tar.zst>` and OCI variants.
- `uds bundle deploy <bundle.tar.zst>` and OCI variants.
- `uds bundle reconfigure <bundle.tar.zst>` and OCI variants for the source
  artifact input. The replacement defaults file is trusted user input.
- `uds bundle pull <oci-reference>` while it materializes pulled content into a
  local artifact.
- `uds bundle push <bundle.tar.zst> <oci-reference>` while it extracts the local
  artifact before upload.

Future commands that process untrusted archives, packages, OCI layouts, HCL,
values data, or created bundle artifacts use this same boundary.

For these commands, untrusted content may name paths for extraction, OCI layer
materialization, values materialization, package staging, reads, writes, links,
copies, renames, or deletes. The command must resolve those paths inside the
root selected for that operation before using them. This applies to temporary
workspaces and package staging directories, not only the first extraction step.
`..`, absolute paths, symlinks, and hardlinks must not escape that root. If
untrusted content creates a symlink or hardlink, the link target must stay
inside the same root or the link must be rejected.

We will implement workspace jails with the Go standard library [`os.Root`](https://pkg.go.dev/os#Root)
where possible. When untrusted content requests a symlink or hardlink, we will
validate the target stays inside the selected root or reject the link. If
`os.Root` does not fit a call site, we will implement one shared helper with
tests for `..`, absolute paths, symlinks, and hardlinks instead of ad hoc path
joins.

### Trusted authoring commands

These commands treat local source input as trusted authoring material:

- `uds bundle create [directory]`.
- `uds bundle dev deploy [bundle-definition]`.
- Source-based operations that parse local `bundle.uds.hcl`, such as
  `uds bundle remove`.

Trusted authoring commands may read files outside the current working directory
or outside the bundle directory when the user or bundle author names those paths.
Explicit authoring paths may use absolute paths, `..` traversal, symlinks, and
hardlinks. Allowed paths include:

- CLI paths such as bundle path, `--config`, `--defaults`, `--output-dir`, and
  `--tmp-dir`.
- `file()` calls in file-backed `bundle.uds.hcl`, `defaults.uds.hcl`, and
  `config.uds.hcl`.
- `values_files` entries in bundle definitions.
- local package `source` entries.

Only the author-selected paths are trusted. If those paths point to archives,
packages, OCI layouts, or other packaged content, paths inside that content use
the untrusted-content boundary.

Users should run trusted authoring commands only against source trees and HCL
that they trust.

## Consequences

### Positive

- Malicious content cannot use path traversal, symlinks, or hardlinks to make a
  command read, write, link, rename, or delete files outside the selected root.
- Users can inspect or deploy created artifacts without giving those artifacts
  access to host files outside the selected root.
- Bundle authors keep existing monorepo and shared-file workflows.
- Code review can reject unjailed untrusted-content path joins.
- Security scanning can treat untrusted content commands as malicious-input
  boundaries and authoring commands as explicit local-file access workflows.

### Negative

- Untrusted-content paths need additional protection code with `os.Root` or a
  shared jail helper.
- Untrusted-content path handling needs tests for traversal and link escape
  cases.

### Neutral

- Existing authoring workflows that use absolute paths, `..`, symlinks, or
  hardlinks remain supported for trusted input.
