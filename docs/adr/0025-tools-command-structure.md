# 25. Tool Command Structure

Dat: 2026-09-03

## Status

Accepted

## Context

As of today, the command `uds tools` ships only one tool - zarf.
Using any of the tools now requires a long invocation such as `uds tools zarf tools kubectl`.
However, zarf is also exposed as `uds zarf` which is required to use when invoking a tool zarf vendors (kubectl, helm).

## Decision

Remove `uds tools zarf` in favor of just having `uds zarf`.
Tools will still exist as a sub command to expose tools and utilities to improve DevX when interacting with the UDS ecosystem.

Some potential examples of tools that would fit are:
```
uds tools kubectl
uds tools helm
uds tools gen-key
uds tools tofu
```

If the tool is directly part of the UDS ecosystem it should not live in tools. Some examples of this are `uds core` & `uds bundles`.

## Alternatives Considered

#### Alias `zarf tools`

Rejected because as cli "is the command-line DevX for interacting with the UDS ecosystem" we may need to expose other tools to improve DevX.

####  Split each tool into a separate command

Rejected because while `uds kubectl`, if we add smaller tools like `gen-key` then the root command namespace would quickly balloon and be hard for users to understand.
Additionally, it provides a nice barrier of what is first party tooling and what is vendored tooling to users.

#### Remove zarf from the uds cli

Rejected users may prefer only having to keep one software up to date.
This also becomes beneficial with only needing to bring one binary over into air-gapped environments.

## Consequences

### Positive

* Users no longer risk invoking `uds tools zarf tools` and getting confusing errors with certain tools not working.
* By no longer hiding `uds zarf` users are encouraged to use a shorter command path.

### Negative

* The full benefits of this ADR will not be realized unless we implement additional tools.
* Removing `uds tools zarf` is a breaking change