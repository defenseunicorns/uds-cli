# 19. Explicit development and artifact bundle deployment

Date: 2026-07-30

## Status

Accepted

## Context

`uds bundle deploy` accepts both mutable bundle authoring input and created bundle artifacts. A directory or `bundle.uds.hcl` is deployed without first creating an immutable bundle artifact. A local `.tar.zst` artifact is extracted, verified through its OCI digest chain, and deployed from embedded package content.

These inputs have different security properties. A mutable source definition has no complete bundle artifact whose provenance or signature can be verified. A created local or OCI artifact has an immutable content graph and is the boundary where bundle integrity and future signature verification apply. Keeping both paths behind one production-facing command makes the unverified development path implicit.

[ADR-0009](0009-bundle-deploy-from-artifact.md) placed source and artifact inputs on the same command and characterized every direct CLI deployment as a development, testing, or CI workflow. The project now needs direct artifact deployment as a supported production workflow in addition to library use by Tofu Provider and Remote Agent. This does not change the pre-1.0 maturity of UDS CLI Next.

## Decision

### Command surface

Mutable source definitions use an explicit development command:

```text
uds bundle dev deploy [bundle-definition]
```

The optional argument accepts a directory containing `bundle.uds.hcl` or a direct `bundle.uds.hcl` path. An omitted argument defaults to the current directory. Development deployment does not create an intermediate bundle artifact and emits an unfiltered stderr diagnostic that bundle provenance and bundle-signature verification are unavailable.

Created artifacts use the production-facing command:

```text
uds bundle deploy <bundle.tar.zst>
uds bundle deploy <oci-reference>
```

The artifact argument is required. Source directories and HCL definitions are rejected with guidance to use `uds bundle dev deploy`.

The `dev` segment is a context group under the bundle resource. It qualifies deployment as an authoring workflow; it does not add Zarf connected-mode behavior or create Zarf packages from source.

### Shared deployment logic

Both commands use the same Cobra-free deployment pipeline for configuration, HCL parsing, bundle validation, package selection, dependency safety, prompting, DAG orchestration, package deployment, and result generation. Cobra commands only complete and validate route-specific inputs, acquire the deploy source, emit route-specific diagnostics, and print the returned result.

### OCI artifact acquisition

OCI deploy composes the existing `Puller.PullBundle` library operation with local artifact deployment:

```text
OCI reference
  -> pull the selected architecture to an operation-owned temporary .tar.zst
  -> extract and verify through the local artifact path
  -> deploy embedded package content
  -> remove the temporary archive and workspaces
```

OCI deploy does not invoke the pull Cobra command. One deploy operation produces at most one confirmation prompt and one `DeployResult`; it does not print an intermediate `PullResult`.

### Verification boundary

Local and OCI artifacts pass through the existing `ExtractArtifact` digest and size verification before materialization or package deployment. OCI deployment does not add a separate loader or bypass the local artifact path.

Development deployment has no bundle artifact to verify. Bundle-signature verification must never be presented as having occurred for this path. Package verification remains a separate concern and can apply to the built Zarf packages referenced by a development bundle when that feature lands.

### Production scope

Direct local and OCI artifact deployment is a supported production workflow using the same deployment library available to Tofu Provider and Remote Agent. Those systems can still provide additional state management, coordination, and fleet capabilities, but they are not mandatory for a production artifact deployment.

UDS CLI Next remains alpha and pre-1.0 until the project lifecycle policy changes. This decision defines the intended deployment boundary, not a general-availability declaration.

## Superseded decisions

This ADR supersedes the following parts of [ADR-0009](0009-bundle-deploy-from-artifact.md):

- The statement in "Scope and Intent" that every direct CLI deployment is limited to development, testing, and CI.
- "Deploy from OCI Reference" being future work.
- The combined source, local artifact, and future OCI input surface under one `uds bundle deploy` command.

This ADR extends [ADR-0003](0003-cli-command-structuring.md) with the `dev` context group beneath the bundle resource. It does not change that ADR's general resource-first command organization.

The following decisions remain in force:

- ADR-0009 artifact extraction, digest verification, materialization, config precedence, and package-loader behavior.
- [ADR-0010](0010-values-file-handling-for-bundle-deploy-from-artifact.md) artifact values-file handling.
- [ADR-0015](0015-multi-arch-bundle-distribution.md) OCI architecture selection and digest-pinned child references.
- [ADR-0004](0004-logging-and-output-strategy.md) result-only stdout and diagnostic stderr.
- [ADR-0005](0005-interactivity.md) one UDS-level confirmation prompt with non-interactive defaults.

## Consequences

### Positive

- The command name communicates whether the input is mutable authoring material or a created artifact.
- Future bundle verification can be enforced at every artifact consumption route without special-casing source definitions.
- Direct OCI deployment reuses existing Pull and artifact deployment behavior.
- CLI, Tofu Provider, and Remote Agent can share one production deployment library.
- Development deploy remains fast and preserves source iteration behavior.

### Negative

- Existing scripts that pass source definitions to `uds bundle deploy` must move to `uds bundle dev deploy`.
- OCI deploy performs network acquisition and artifact verification before it can show a bundle preview or deployment confirmation.
- The command layer must resolve acquisition options before embedded defaults are available, then merge embedded variables without changing config precedence.

### Neutral

- Bundle signing, signature transport, trust policy, and bypass flags are implemented separately.
- Package signing and package verification are implemented separately.
- Tofu Provider and Remote Agent remain preferred where their additional orchestration capabilities are required.

## Alternatives considered

### Keep automatic source and artifact dispatch

Rejected. It preserves an implicit mutable and unverifiable path on the production-facing command and complicates secure defaults.

### Create a temporary artifact for every development deploy

Rejected. It adds build cost to the authoring loop and still does not establish trusted provenance unless the artifact is signed by a trusted identity.

### Implement a separate OCI deployment engine

Rejected. Pull already produces the local artifact format, and local artifact deployment already verifies and materializes that format. A second path would duplicate security-sensitive behavior.

### Require Tofu Provider or Remote Agent for production

Rejected. The shared deployment library is production-capable, and direct artifact deployment is a valid workflow when external state management or fleet coordination is unnecessary.
