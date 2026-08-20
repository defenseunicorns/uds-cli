# 12. Next Mode Transition

Date: 20 August 2026

## Status

Accepted

## Context

UDS CLI is transitioning from the existing Legacy implementation to the imported Next implementation. Existing users need stable default behavior while Next matures. Contributors also need a clear rule for where new work belongs and how long Legacy remains a first-class development target.

## Decision

UDS CLI supports two modes during the transition:

- **Legacy mode** is the default during alpha and preserves existing behavior.
- **Next mode** is available as an alpha preview through the `NextMode` feature flag.
- Next mode is expected to mature during alpha and become the default mode in beta.
- Legacy mode will be removed after the beta migration window.

Next mode is selected with either of the following:

```bash
uds --features=NextMode=true <command>
CLI_FEATURES=NextMode=true uds <command>
```

New feature work should target the canonical Next packages unless a maintainer explicitly scopes the change to Legacy. Legacy code remains available for bug fixes, compatibility fixes, and retained non-bundle commands needed during the transition.

## Consequences

- The `uds` binary remains a single executable with mode selection at startup.
- Legacy remains safe by default for alpha users.
- Next can be exercised in CI and by early adopters without changing the default user path.
- Contributors should avoid adding new product capabilities to Legacy unless required for compatibility.
- Documentation must label Next behavior as alpha until it becomes the default in beta.
