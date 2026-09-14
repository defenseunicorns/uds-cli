# 24. UDS Core Operator Monitor Command Placement

Date: 2026-08-28

## Status

Accepted

## Context

[ADR 0003](./0003-cli-command-structuring.md) establishes the UDS CLI Next command pattern as `uds <component> <resource> <action>`, with optional default component and default action positions. One example in that ADR places Pepr monitoring at:

```bash
uds core pepr monitor
```

This ADR supersedes that example without changing ADR 0003's general command-structuring decision.

The legacy command is:

```bash
uds monitor pepr
```

That command primarily exists to observe UDS Core operator behavior and the related Pepr policy activity that explains what the operator is doing in the cluster. Although Pepr is the underlying implementation technology, the user-facing resource of interest is the UDS Core operator.

Keeping `pepr` as the resource exposes implementation detail as the primary command noun. Placing the command under `core operator` makes the command read as a UDS Core operation while preserving ADR 0003's component-resource-action structure.

## Decision

Expose operator monitoring as:

```bash
uds core operator monitor
```

This keeps `core` as the component, `operator` as the resource, and `monitor` as the action. Pepr remains an implementation detail of the monitor output and underlying data source rather than the command resource.

## Consequences

### Positive

- Preserves the full component-resource-action shape from ADR 0003
- Names the user-facing resource as the UDS Core operator instead of the Pepr implementation
- Leaves room for future UDS Core operator actions under `uds core operator <action>`
- Avoids creating a root-level `pepr` resource that may imply Pepr is a standalone UDS CLI domain

### Negative

- Longer than both the legacy `uds monitor pepr` command and a root-level `uds pepr monitor` command
- Users familiar with the legacy command need to learn the new resource placement

### Neutral

- The monitor output may still use Pepr-specific event terms such as `ALLOWED`, `DENIED`, and `MUTATED`
- `uds core pepr monitor` is not used, despite appearing as the original example in ADR 0003
