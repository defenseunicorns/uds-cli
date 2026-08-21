# 10. Values File Handling for Bundle Deploy from Artifact

Date: 2026-04-29

## Status

Accepted

## Context

[ADR-0007](0007-bundle-definition-and-value-files-in-oci-layout.md) established that values files are materialized into the bundle OCI layout at create time as content-addressed layers under the bundle definition manifest. Each layer is annotated with `org.opencontainers.image.title` of the form `values/<package-name>/<priority-index>.yaml` (e.g. `values/monitoring/0.yaml`, `values/monitoring/1.yaml`). This normalization resolves relative and external paths into a self-contained artifact and preserves the user-defined priority order from each package's `values_files` HCL field.

[ADR-0009](0009-bundle-deploy-from-artifact.md) established the artifact deploy flow. After extraction, values file blobs are materialized to disk at `<workspace>/values/<pkg>/<idx>.yaml`. ADR-0009's implementation plan proposed a `DeployOptions.ValuesFiles` map (package name → ordered file paths) wired from `CollectExtractedValuesFiles`, bypassing HCL `values_files` resolution entirely for artifact deploys. Decision 1 below refines this into two concrete options.

ADR-0009 punted on two questions about the relationship between the original `values_files` field on each `package` block and the materialized `values/<pkg>/<idx>.yaml` layers. Those questions need answers before the artifact-based deploy is implemented:

1. The `bundle.uds.hcl` file within the bundle artifact still references original paths in the `values_files` field. Those paths may be relative or absolute, and may resolve to locations outside the bundle directory. During artifact deploy, neither form can be guaranteed valid: the original source tree may be gone, and absolute paths are environment-specific and cannot be reproduced in an arbitrary deploy environment. During bundle source-directory deploy, each path is resolved against the bundle directory if relative, or used as-is if absolute. These paths must exist in order for the `bundle create` and `bundle deploy` (from bundle source-directory) to succeed.

2. The priority index baked into each values filename (`0.yaml`, `1.yaml`, ...) is authoritative at the artifact level by ADR-0007 design. It is not yet specified whether the deployer reads ordering from the filenames or re-derives it from the HCL `values_files` slice at deploy time.

These decisions shape how the deployer's existing values pipeline (resolve → template → `value.ParseFiles`) is fed for artifact deploys, and whether the deploy path requires divergent handling of values files on artifact-vs-source to apply.

### Zarf Values File Handling (Reference)

Zarf's own `zarf package create` provides a useful analogue. During create, every values file is copied or downloaded into `components/<name>.tar` under a `values/` directory and renamed to an index-based filename via `StandardValuesName`: `{chartName}-{chartVersion}-{idx}` (e.g., `podinfo-6.11.2-0`). A Zarf package tarball therefore contains two forms of `zarf.yaml`: the original at the top level of the tarball (author-facing, with original `valuesFiles` paths), and an embedded copy inside each `components/<name>.tar` (deploy-facing). For normal (non-skeleton) packages both copies retain the original `valuesFiles` list - `PackageChart` saves and restores `chart.ValuesFiles` around the copy step, so the struct is never mutated. At deploy time, Zarf ignores the original paths in `zarf.yaml` and uses `StandardValuesName` with the position index to locate each renamed file.

For skeleton packages (created via `zarf package publish <dir> oci://...`), `assembleSkeletonComponent` mutates `chart.ValuesFiles` in place to the index-based relative paths before writing the embedded `zarf.yaml` inside `components/<name>.tar`, so the deploy-facing definition reflects the renamed files. The top-level `zarf.yaml` in the tarball retains the original paths.

The net result: Zarf's package format uses index-based filenames in the filesystem layout and resolves them by index at deploy time. The original `valuesFiles` entries in a normal package's embedded `zarf.yaml` are display-only artifacts of the build; they are not load-bearing at deploy time. `zarf package inspect definition` against a normal package displays the original paths; against a skeleton, it displays index-based paths.

### Bundle Inspect Behavior

The `bundle inspect` command displays the `values_files` paths from `bundle.uds.hcl` as part of each package summary (`pkg/bundle/printer.go:124`). For bundle source-directory inspection, this is unambiguous - the paths exist on disk and the user can open them.

For bundle artifact inspection, two options exist:

- **Original paths**: communicates author intent; paths may be recognizable from the source repository; but they are not navigable within the artifact. A path like `/home/author/projects/team/monitoring-override.yaml` tells the artifact user nothing actionable.
- **Index-based paths**: correspond to actual files in the artifact layout; a user who extracts the artifact can locate and read them directly. `values/monitoring/0.yaml` exists in the extracted workspace.

Bundle authors maintaining the source may find original paths useful - they reflect the actual file structure in the repository. Bundle artifact consumers may not have source access; index-based paths are the only ones they can navigate.

## Decision

### 1. Bundle artifact deploy bypasses bundle definition HCL `values_files` - use materialized values directly

For bundle artifact deploys, the deployer bypasses bundle definition HCL `values_files` resolution entirely and reads values from the materialized `<workspace>/values/<pkg>/<idx>.yaml` tree. `CollectExtractedValuesFiles` walks that tree and returns a `map[string][]string` (package name → ordered paths). Every package's values files are sourced exclusively from the materialized workspace - no original bundle definition HCL `values_files` path is used, regardless of whether it happens to resolve in the deploy environment. The `bundle.uds.hcl` embedded in the artifact is stored verbatim - original `values_files` paths are not rewritten. Source-directory deploys do not mutate `pkg.ValuesFiles`.

Two implementation options exist for wiring these paths into the deploy pipeline:

**Option 1 (preferred): In-memory `pkg.ValuesFiles` mutation**

After `ExtractArtifact` and `ParseBundleFile`, the artifact deploy `Run()` overwrites each package's `ValuesFiles` slice with workspace-relative materialized paths before constructing deploy options:

```go
materializedValues := CollectExtractedValuesFiles(workspaceDir)
for i := range bundle.Packages {
    // Always overwrite - packages absent from the map get nil, clearing any stale HCL paths.
    bundle.Packages[i].ValuesFiles = materializedValues[bundle.Packages[i].Name]
}
opts.BundleDir = workspaceDir
// ZarfDeployer.prepareValuesAndVariables runs unchanged:
// resolveValuesFiles(pkg.ValuesFiles, opts.BundleDir) joins workspace dir + relative paths
```

`ZarfDeployer` and `prepareValuesAndVariables` are untouched - `resolveValuesFiles` joins `opts.BundleDir` against the workspace-relative paths and finds the materialized files. No new fields on `DeployOptions` or `DeployPackageOptions`, no branching in `prepareValuesAndVariables`.

**Option 2 (fallback): Abstract resolving of values files**

If Option 1 proves insufficient (e.g., the bundle struct is shared across calls and mutation is unsafe), introduce a resolver interface:

```go
type ValuesFilesResolver interface {
    ResolveValuesFiles(pkg *Package) ([]string, error)
}
```

Two implementations:

- `HCLValuesFilesResolver`: resolves `pkg.ValuesFiles` relative to `BundleDir` (source-directory deploy)
- `MaterializedValuesFilesResolver`: walks `<workspace>/values/<pkg>/*.yaml` lexically (artifact deploy)

`prepareValuesAndVariables` accepts a `ValuesFilesResolver` and calls it in place of `resolveValuesFiles`. Source-directory deploys pass `HCLValuesFilesResolver`; artifact deploys pass `MaterializedValuesFilesResolver`. This keeps the bundle struct immutable at the cost of adding the interface and two implementations.

**Why Option 1 is preferred:**

- No new types, no interface plumbing, no changes to `prepareValuesAndVariables` or `DeployOptions`.
- The bundle struct parsed from the extracted workspace HCL is ephemeral (constructed per-deploy invocation); mutation is safe.
- `resolveValuesFiles` already handles the path-joining behavior needed; the only change is what paths are in `pkg.ValuesFiles` when it is called.

**Why:**

- Original bundle definition HCL values file paths cannot be assumed valid at bundle artifact deploy time. They may be relative to a source tree that no longer exists, or absolute paths that are environment-specific. The materialized layers are the artifact's canonical, content-addressed record.
- Consistent with Zarf's normal package behavior: at deploy time, Zarf ignores the original `valuesFiles` paths in `zarf.yaml` and uses `StandardValuesName` with the position index to locate each renamed file. The original paths are informational, not load-bearing.
- Preserves original paths in the stored HCL, keeping them available for authors and for inspect (see below).

### 2. Filename index is authoritative for ordering

`CollectExtractedValuesFiles` sorts entries within each package directory by numeric index parsed from the filename (`strconv.Atoi` on the stem before `.yaml`). `0.yaml` is the base; higher indices override. The bundle definition HCL `values_files` slice is not consulted for ordering at artifact deploy time.

**Why:**

- ADR-0007 pinned the index-equals-priority contract at create time. The filename is the order; re-deriving from HCL adds a redundant lookup that can only diverge.
- Numeric sort is correct for all index ranges - lexical sort breaks at ≥10 files per package (`10.yaml` sorts before `2.yaml`). Parsing the integer from the filename stem handles arbitrarily wide index ranges without zero-padding.
- The digest chain covers layer titles (annotations are part of the manifest blob). Tampering that reorders files changes a manifest digest and is caught by verification (ADR-0009 §3).

### 3. Inspect displays original HCL paths

`inspect` displays `pkg.ValuesFiles` from the embedded `bundle.uds.hcl` as the user-facing record of which files the author included. Since the stored HCL is not rewritten, both source-directory and artifact inspect show the original author paths.

- **Source-directory inspect**: paths exist on disk and are navigable.
- **Artifact inspect** (future): paths reflect the author's source repository structure. They are not navigable within the artifact but communicate authoring intent and are recognizable to maintainers familiar with the source.

Both cases use the same `ParseBundleFile` → `ToInspectResult` → `BufferString` pipeline with no branching.

## Alternatives Considered

### A. Create-time bundle definition HCL rewrite - store index-based paths in artifact bundle definition HCL

During `uds bundle create`, after materializing values files as indexed OCI layers, rewrite the `values_files` slice in `bundle.uds.hcl` before storing it as an OCI layer. Each original path is replaced with the corresponding index-based path:

```hcl
# Source bundle.uds.hcl (untouched on disk)
values_files = ["../shared/monitoring.yaml", "overrides/local.yaml"]

# Stored in artifact OCI layer
values_files = ["values/monitoring/0.yaml", "values/monitoring/1.yaml"]
```

**Implementation:**

- The create pipeline rewrites `values_files` in-memory on a copy of the parsed bundle struct before serializing `bundle.uds.hcl` as an OCI layer. The on-disk source bundle definition HCL is not touched.
- `ExtractArtifact` materializes the rewritten bundle definition HCL at `<workspace>/bundle.uds.hcl`. The existing source-directory deploy pipeline reads it, resolves `values/monitoring/0.yaml` relative to `<workspace>`, and finds the materialized file.
- No `DeployOptions.ValuesFiles` map, no `CollectExtractedValuesFiles` walk, no branch in `prepareValuesAndVariables` - the artifact deploy path is identical to the source-directory path.
- Artifact inspect displays `values/monitoring/0.yaml` - navigable for consumers who extract the artifact.

**Why not chosen:**

- Stored bundle definition HCL diverges from the source. Authors who extract the artifact and read the embedded `bundle.uds.hcl` see unfamiliar index-based paths instead of the file names they wrote. Optimizes for artifact consumers at the cost of author familiarity.
- Inspect output for artifact bundles shows index-based paths that differ from source-directory inspect. This inconsistency may confuse bundle maintainers comparing inspect output across deploy modes.
- Adds create pipeline complexity: `createBundleDefinitionManifest` must mutate a copy of the parsed bundle struct before serializing.
- Index-based file references may be of limited value to artifact consumers. A future deploy dry-run / preview mode (see Future Scope) that renders the fully resolved Helm values would be more actionable than navigating to raw values files - making the primary justification for this alternative obsolete.

### B. Re-derive ordering from bundle definition HCL at deploy time

Read each package's `values_files` slice from the embedded HCL and use its list order, ignoring the filename index.

- **Pro:** A single canonical source for ordering (the HCL).
- **Con:** Two sources of truth at the artifact level - the filename index and the HCL slice - that must agree. Divergence would indicate a bug but the deployer would still need to handle it.
- **Con:** The artifact layer order is self-describing per ADR-0007. Requiring HCL to interpret the materialized layout order adds unnecessary coupling.

### C. Restrict `values_files` paths within bundle definition HCL to bundle directory - obsolete index-based renaming

Reject any `values_files` entry at create time that is absolute or escapes the bundle directory. With this constraint, original paths could be preserved verbatim as OCI layer titles (e.g., `values/monitoring-override.yaml`) and materialized to the same relative location in the workspace. The deploy pipeline would resolve the original HCL paths against the workspace root unchanged, just as it does for source-directory deploys today.

- **Pro:** Obsoletes ADR-0007's index-based renaming and the priority-by-filename contract. The path the author wrote is the path on disk in both the source tree and the extracted artifact.
- **Pro:** Collapses Decision 1's bypass - original HCL paths resolve correctly against the workspace root, eliminating `pkg.ValuesFiles` mutation and the artifact-vs-source asymmetry.
- **Pro:** Artifact inspect output is fully navigable - paths shown match files on disk after extraction.
- **Pro:** Simplifies both create (no rename, no priority indexing) and deploy (single resolution path) logic.
- **Con:** Values files outside the bundle directory can no longer be referenced. This is a confirmed real-world pattern: bundles use relative paths like `../common-files/values.yaml` to share values across packages in a monorepo, or collocate all values files in a single directory (e.g., `../values/`) separate from the bundle definition. Although this is a Zarf package example, it is reasonable to expect that this pattern may be applied to bundle authoring.
- **Con:** Requires changes to ADR-0007's layer title scheme and create-time validation. Not in scope for the current artifact deploy work.
- **Why not chosen:** External `values_files` paths are a confirmed, intentional authoring pattern for Zarf packages and is reasonable to expect this pattern to be applied to bundles.

## Consequences

### Positive

- **Stored HCL preserves original author paths.** Authors who extract the artifact and read the embedded `bundle.uds.hcl` see the same `values_files` entries they wrote. Familiar to maintainers; consistent with Zarf's normal package behavior where original `valuesFiles` paths are retained in the embedded `zarf.yaml`.
- **ADR-0007 unchanged.** The layer title scheme (`values/<pkg>/<idx>.yaml`) and create-time ordering contract stand. The stored HCL is not modified.
- **No create pipeline mutation.** `createBundleDefinitionManifest` stores `bundle.uds.hcl` verbatim; no in-memory rewrite of the parsed bundle struct is needed.
- **Consistent inspect output across deploy modes.** Both source-directory and artifact inspect display the same original paths. Bundle maintainers see consistent output regardless of whether they inspect from source or artifact.

### Negative

- **Minor asymmetry in deploy paths.** Artifact-deploy `Run()` mutates `pkg.ValuesFiles` with workspace-relative paths after extraction; source-directory deploy does not. This asymmetry is contained to the `Run()` method and does not propagate to `ZarfDeployer` or `prepareValuesAndVariables`. If the mutation approach is ever untenable, Option 2 (`ValuesFilesResolver` interface) is the escape hatch - at the cost of introducing a new interface and two implementations.
- **Artifact inspect shows non-navigable paths.** A user inspecting a `.tar.zst` artifact sees original paths that may not exist in the deploy environment. Locating the actual deployed values files requires knowledge of the `values/<pkg>/<idx>.yaml` index mapping documented in ADR-0007.

### Neutral

- **Inspect behavior is consistent but limited for artifact consumers.** Original paths communicate authoring intent and are recognizable from the source repository, but are not directly usable by consumers who only have the artifact.

## Future Scope

- **Bundle artifact inspect command.** Adding `uds bundle inspect <bundle>.tar.zst` means extracting the stored HCL and passing it through the same `ParseBundleFile` → `ToInspectResult` → `BufferString` pipeline. The stored HCL has original paths; no special handling needed.
- **Deploy dry-run / preview mode.** Rather than showing values file references, a `uds bundle deploy --dry-run` (or `--preview`) flag would run the full values resolution pipeline - extract values files, template `{{ .vars.* }}` with config variables, deep-merge via `value.ParseFiles` - and display the resulting rendered Helm values per package without applying them to the cluster. Output format would mirror `inspect` but replace `Value Files` with the actual rendered values. This is similar behavior to uds-cli v0. For bundle artifact consumers who cannot navigate original file references, rendered values are more actionable than either original paths or index-based paths. Open design questions before implementation: handling of sensitive values in output (masking, opt-in flag, or explicit warning), whether `--config` is required or optional in this mode, and whether output should be machine-readable (YAML/JSON) in addition to human-readable. If implemented, this may reduce or eliminate the value of surfacing values file references in artifact inspect entirely.
