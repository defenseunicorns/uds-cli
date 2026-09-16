# 27. Resume bundle deploy

Date: 2026-09-16

## Status

Accepted. This supersedes ADR-0009's statement that bundle deploy resume is future work.

## Context

An interrupted or repeated deployment should avoid redeploying packages that Zarf has already recorded as the exact intended deployment. The decision must not turn deploy into reconciliation, read mutable artifact sources while deploying an artifact, or download complete packages merely to decide whether to skip them.

## Decision

Next-mode `uds bundle deploy` and `uds bundle dev deploy` accept `--resume` (`-r`). Package selection and dependency-safety validation run first. Bundle `PreDeploy` hooks then finalize config and package hooks. Resume reads Zarf deployed-package state once, filters the selected DAG in memory, and runs the normal orchestration.

Resume identifies a package by Zarf `metadata.name` and the bundle namespace override, never the HCL package label. It skips only one unambiguous deployed record whose digest is non-empty and equal to the intended digest, whose component-name set exactly matches the intended post-filter set, and whose components all have `Succeeded` status. Missing state and every mismatch redeploy. Duplicate package records or component names are mismatches.

The deployment component filter is also the resume filter. A state read, intended definition read, or intended digest read error aborts after bundle `PreDeploy` and before package deployment. Resume rejects package `PreDeploy` hooks because they can mutate the effective deployment after the skip decision. When all selected packages match, hooks still run and the result has an empty package list. With resume disabled, deploy performs no state or intended-spec reads.

Intended identity loading is metadata-only:

- OCI sources resolve a root descriptor, read its root manifest and schema-aware `zarf.yaml`, apply the deployment filter, and retain that resolved root digest for deployment.
- Extracted artifacts read the indexed embedded package manifest and `zarf.yaml` from the verified OCI store. They never fall back to the original source.
- Local directories and archives use Zarf's schema-aware load APIs without deployment staging.

## Consequences

### Positive

- Repeated deploys avoid package work only when Zarf has recorded an exact successful match.
- One cluster-state list replaces per-package cluster reads.
- Artifact resume remains inside the verified artifact boundary.
- OCI deployment records the same resolved digest used for the resume decision.

### Negative

- Resume needs cluster read access and can fail after bundle `PreDeploy` if state or metadata cannot be read.
- Resume cannot be combined with package `PreDeploy` hooks.
- Values-file, deploy-time configuration, and rendered-values changes are not represented in the match.
- Mutable development sources may change after the decision; resume does not snapshot them.

## Alternatives considered

### Per-package deployed-state reads

Rejected. They create a cluster per package, add round trips, and make a single decision observe inconsistent state.

### Stage complete packages before matching

Rejected. Resume needs metadata and a digest, not package payloads; staging makes skipped packages unnecessarily expensive.

### Compare HCL package labels or only digests

Rejected. Zarf persists metadata names and namespace overrides, and digest-only matching misses partial or failed component deployments.

### Local checkpoints or reconciliation

Rejected. They introduce another state authority and broader recovery semantics than a narrow Zarf-state optimization.

## Non-goals

- Resume for remove, retries, or reconciliation loops.
- Detecting values-only changes.
- A generic package-filter framework.
- Snapshotting mutable development sources between matching and deployment.
