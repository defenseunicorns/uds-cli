# 25. Tool Command Structure

Date: 2026-09-03

## Status

Accepted

## Context

As of today, the command `uds tools` ships only one tool - Zarf.
The current recommented path to use Zarf-vendored tools requires a long invocation such as `uds tools zarf tools `.
However, Zarf is also exposed as `uds zarf`, which is required when invoking certain commands in tools Zarf vendors, such as kubectl or monitor.

## Decision

Remove `uds tools zarf` in favor of just having `uds zarf`.
Tools will still exist as a subcommand to expose tools and utilities to improve DevX when interacting with the UDS ecosystem.

Potential examples of tools that would fit this namespace include:
```
uds tools kubectl
uds tools helm
uds tools gen-key
uds tools tofu
```

If the tool is directly part of the UDS ecosystem it should not live in tools. Some examples of this are `uds core` and `uds bundle`.

## Alternatives Considered

#### Alias `zarf tools`

Rejected because UDS CLI is "the command-line DevX for interacting with the UDS ecosystem" and may need to expose other tools to improve DevX.

#### Split each tool into a separate command

Rejected because while `uds kubectl` may be reasonable, adding smaller tools like `gen-key` would quickly balloon the root command namespace and make it harder for users to understand.
Additionally, `uds tools` provides a clear boundary between first-party UDS commands and vendored tooling.

#### Remove Zarf from UDS CLI

Rejected because users may prefer only having to keep one piece of software up to date.
This also becomes beneficial by requiring only one binary to be brought into air-gapped environments.

## Consequences

### Positive

* Users no longer risk invoking `uds tools zarf tools` and getting confusing errors with certain tools not working.
* By no longer hiding `uds zarf` users are encouraged to use a shorter command path.

### Negative

* The full benefits of this ADR will not be realized unless we implement additional tools.
* Removing `uds tools zarf` is a breaking change.
