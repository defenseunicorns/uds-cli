# 5. Interactivity

Date: 2026-02-27

## Status

Accepted

Amended 2026-06-08: `--prompt` scope narrowed to UDS-level pre-flight confirmation only. See [Changelog](#changelog).

## Changelog

### 2026-06-08 - Narrowed `--prompt` scope to UDS pre-flight confirmation

**Context:** The original decision left open whether `--prompt` would also enable Zarf's own interactive prompts (component selection, variable resolution). In practice, `ZarfDeployer.DeployPackage` passed `opts.Prompt` through to Zarf's `DeployOptions.IsInteractive`, giving `--prompt` two distinct effects: a UDS-level bundle confirmation and Zarf-level per-package prompts.

**Change:** `IsInteractive` in `ZarfDeployer.DeployPackage` is now hardcoded to `false`. The `Prompt` field was removed from `DeployOptions` and `DeployPackageOptions`. `--prompt` is now a global flag that triggers a single UDS pre-flight confirmation at the bundle level before any package deployment begins. Zarf's own interactive prompts are always disabled regardless of `--prompt`.

**Rationale:** Passing `--prompt` through to Zarf created an incompatibility with parallel deployment: Zarf's interactive mode requires serialized input, forcing `--concurrency=1` whenever `--prompt` was set. The narrowed scope lets `--prompt` and `--concurrency` remain independent, and makes the "ask once, then proceed non-interactively" behavior explicit and predictable for all four personas.

## Context

UDS CLI Next serves multiple personas with different interactivity needs:

- **Pipeline / Automation Operators** run the CLI in CI/CD pipelines where interactive prompts cause hung jobs. They need deterministic, scriptable behavior with stable exit codes and machine-readable output.
- **Delivery Engineers** build and iterate on bundles. They benefit from safety confirmations on destructive actions but otherwise optimize for speed and repeatability.
- **Platform Engineers** prefer composability (pipes, JSON, grep) and view interactive prompts as friction.
- **Mission Operators** are often not Kubernetes experts and benefit from guided flows for uncommon or risky tasks, but still rely on runbook-style automation.

The previous CLI was **interactive by default** and was designed to be used in a human-in-the-loop context. The new CLI is being designed with more automation in mind, where interactivity is not necessary.

## Decision

The CLI will be **non-interactive by default**. A `--prompt` flag will be available to opt into interactive mode for commands where guided input is useful (e.g., selecting resources, confirming destructive actions).

## Consequences

### Positive

- **Pipeline Safety**: Non-interactive default ensures the CLI works reliably in CI/CD without hung jobs or unexpected prompts
- **Broad Persona Coverage**: All four personas get a safe default; those who want interactivity opt in explicitly
- **Predictable Behavior**: Scripts and automation can depend on deterministic CLI behavior without defensive workarounds
- **Ecosystem alignment**: Most of the tools for Platform Engineers (e.g., kubectl) are non-interactive by default, so this aligns with their expectations and workflows

### Negative

- **Discoverability**: Users who would benefit from guided flows may not know `--prompt` exists
- **Opt-In Friction**: Although Mission Operators are expected to use other tools (e.g. the UDS Android app), some of them will use UDS CLI Next. They will have to remember to pass `--prompt` for interactive guidance

### Neutral

- **Future Expansion**: Individual commands can introduce command-specific interactive behaviors behind `--prompt` without changing the global default
