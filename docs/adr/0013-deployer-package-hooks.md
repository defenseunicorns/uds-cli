# 13. Deployer Package and Bundle Hooks

Date: 2026-06-02

## Status

Accepted.

## Context

The UDS Remote Agent needs to manipulate the Zarf package's `Components[].Images` and `Components[].ImageArchives` before deploy when images are already cross-mounted into the Zarf registry out-of-band. This mutation currently lives in a fork of the deploy path.

Exposing first-class hook extension points in the deploy library lets consumers (UDS Remote Agent, UDS Fleet) customize deploy behavior without forking.

## Decision

### Mechanism

**`PackageDeployHooks`** - per-package callbacks embedded in `DeployPackageOptions`:

```go
type PackageDeployHooks struct {
    PreDeploy  func(ctx context.Context, pkg *Package, pkgLayout *layout.PackageLayout, opts *packager.DeployOptions, packageOpts *DeployPackageOptions) error
    PostDeploy func(ctx context.Context, pkg *Package) error
}
```

**`BundleDeployHooks`** - bundle-scope callbacks on `DeployOptions`, fired once per deploy:

```go
type BundleDeployHooks struct {
    PreDeploy  func(ctx context.Context, b *UDSBundle, opts *DeployOptions) error
    PostDeploy func(ctx context.Context, b *UDSBundle) error
}
```

Both structs are threaded through `DeployOptions`:

```go
type DeployOptions struct {
    // ...existing fields...
    BundleDeployHooks  BundleDeployHooks
    PackageDeployHooks PackageDeployHooks

    // PackageDeployFn replaces the entire per-package deploy. Nil defaults to the
    // deployer's DeployPackage. An override that still wants loader + hooks should
    // set opts.ClusterDeployFn and then delegate to the deployer's DeployPackage.
    PackageDeployFn func(ctx context.Context, pkg *Package, opts DeployPackageOptions) error
}
```

`PackageDeployHooks` is also embedded in `DeployPackageOptions` for callers who use `DeployPackage` directly. `DeployBundle` threads hooks from `DeployOptions` into each package's `DeployPackageOptions`.

`DeployPackageOptions` exposes `ClusterDeployFn`, replacing only the `packager.Deploy` call while leaving the loader and hook pipeline intact:

```go
type DeployPackageOptions struct {
    // ...existing fields...

    // ClusterDeployFn replaces packager.Deploy. Nil defaults to packager.Deploy.
    ClusterDeployFn func(ctx context.Context, pkgLayout *layout.PackageLayout, opts packager.DeployOptions) error
}
```

### Defaults always run

Nil func fields are replaced with no-ops by `withDefaults()` before the first call. Every deploy exercises both hooks for every package and for the bundle.

### Supported mutations in PreDeploy

`PreDeploy` receives live pointers; mutations take effect immediately:

- **`pkgLayout.Pkg.Components[].Images`** - zero to skip image push (only valid for partial packages whose images are reachable via cross-mount).
- **`pkgLayout.Pkg.Components[].ImageArchives`** - same rule.
- **`*packager.DeployOptions`** - `NamespaceOverride`, `SetVariables`, `Values`, `IsInteractive`.
- **`*DeployOptions`** (bundle `PreDeploy` only) - `PackageDeployHooks`, `PackageDeployFn`. Mutations to `Source` and `Packages` have no effect (`Source` is consumed before `PreDeploy`; `Packages` is validated and resolved into the DAG before `PreDeploy`).
- **Do not mutate** `opts.Config` or `opts.Config.Options` - these fields are validated before `PreDeploy` but read afterward (e.g. `Config.Options.Concurrency`), so a mutation takes effect while bypassing validation.

### Ordering and concurrency

- **Ordering**: `Bundle.PreDeploy → (Package.PreDeploy → packager.Deploy → Package.PostDeploy)* → Bundle.PostDeploy`
- Bundle hooks fire once, single-threaded.
- Package hooks may run concurrently across DAG levels (bounded by `Options.Concurrency`). Implementations must be concurrency-safe.
- `PreDeploy` error aborts before `packager.Deploy`; `PostDeploy` is not called.
- `Bundle.PreDeploy` error aborts before any package deploys.
- `Bundle.PostDeploy` error returns after all packages succeed.

### Deploy customization

Two fields give consumers surgical control over the deploy pipeline:

- **`ClusterDeployFn`** - replaces only the `packager.Deploy` call. The full pipeline (layout loading, hooks, values preparation) runs unchanged. Use when a consumer needs a different cluster interaction (e.g. dry-run, remote dispatch) while retaining all pre/post processing.
- **`PackageDeployFn`** - replaces the entire per-package deploy path, bypassing the loader, hooks, and `packager.Deploy`. Use when a consumer owns the full deploy lifecycle and needs no CLI-provided pipeline. An override that still wants loader + hooks should set `ClusterDeployFn` and delegate to the deployer's `DeployPackage`.

Both are nil by default; standard behavior applies when unset.

## Consequences

- Consumers can customize layout and deploy options without forking.
- Image-zeroing for cross-mount is a supported, documented pattern.
- `types.go` gains an import of `github.com/zarf-dev/zarf/src/pkg/packager` (already a transitive dependency).
- `DeployOptions` gains two new fields; existing call sites get no-op defaults at no cost.
