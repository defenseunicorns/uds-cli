# 11. Validation at Public Library Entrypoints

Date: 2026-05-27

## Status

Accepted

Amends [ADR-0002](0002-cli-architecture-patterns.md). ADR-0002's `Complete → Validate → Run` command pattern remains in force for cobra commands. This ADR adds a second validation layer at the public library API surface and narrows the responsibility of `Validate()` accordingly.

Amended 2026-06-08: removed `--prompt + --concurrency > 1` example. See [Changelog](#changelog).

## Changelog

### 2026-06-08 - Remove `--prompt + --concurrency > 1` example

The original draft of this ADR referenced `--prompt` together with `--concurrency > 1` as a flag-combination validation example handled at the CLI layer. That constraint was eliminated when `--prompt` was narrowed to a UDS-level pre-flight confirmation (see [ADR-0005 Changelog](0005-interactivity.md#changelog)): `--prompt` and `--concurrency` are now independent, so no cross-flag validation rule is required.

## Context

First-class library support for deploying Bundles Next in UDS Fleet requires amending the `Complete → Validate → Run` pattern (established by [ADR-0002](0002-cli-architecture-patterns.md)) and performing additional validation at the method level for every public interface (located in the `types.go` files).

This means there are two types of validation performed:

- **Cobra level** - validation of CLI inputs, including flag combinations and file existence checks. This is the existing responsibility of `Validate()` in the cobra command pattern.
- **Public library API level** - validation of inputs to public methods on `Parser`, `Loader`, `Deployer`, `Puller`, and `Pusher` interfaces. This is a new responsibility added by this ADR.

## Decision

Adopt a two-layer validation model. Each layer owns rules involving inputs it can directly see; the layers have disjoint inputs.

### Layer 1: CLI `Validate()` (ADR-0002)

`Validate()` on each cobra command's Options struct owns rules involving **CLI-only inputs**:

- Positional argument count and shape.
- Bundle path existence on the local filesystem (`ValidateBundlePath`). The library layer validates path shape and parseability via `ParseBundleFile` and option struct validation; CLI `ValidateBundlePath` owns filesystem-existence checks and artifact-vs-directory resolution UX provided by cobra's `AllowArtifactBundlePath()`.
- Flag-combination rules involving fields that exist only at the CLI layer.

`Validate()` no longer calls `bundle.ValidateConfig`. The library layer owns config validation.

### Layer 2: Public Library API

Every exported method on a public interface in `pkg/bundle/` validates its own input struct as its first statement. Examples of interfaces and methods covered by this rule:

- `Parser` - `ParseBundleFile`, `ParseBundleBytes`, `ParseBundleConfig`
- `Deployer` - `DeployBundle`, `DeployPackage`

Validators live in `pkg/bundle/validation.go` and follow a naming convention of value-receiver methods on option structs:

- `func (o DeployOptions) Validate() error`
- `func (o DeployPackageOptions) Validate() error`

Each validator:

1. Performs structural checks: nil receivers, non-nil required sub-structs, range checks on scalar fields, simple string-shape checks.
2. Is idempotent: duplicate calls down the call chain are harmless.
3. May perform more extensive checks, including disk I/O, network I/O, and cluster access, when the entrypoint's correctness requires it.

## Consequences

### Positive

- **Library safe by default**: Fleet, Tofu, and Remote Agent cannot reach a nil-dereference by skipping the CLI. Every public method gates its inputs.
- **Disjoint boundaries**: the CLI layer never sees a hand-crafted `UDSBundleConfig`; the library layer never sees a cobra flag. Each layer owns checks for inputs it can see.
- **Single source of truth per options type**: each `Validate()` method is the canonical structural contract for its struct.
- **Cobra-free `Run()` is safe**: once `Run()` is decoupled from cobra, the library layer's defensive validation is the only thing standing between a library caller and a nil panic. This ADR makes that explicit.
- **ADR-0002 preserved for cobra**: the `Complete → Validate → Run` pattern remains intact for cobra commands. Only the scope of `Validate()` narrows.

### Negative

- **Repeated validation down the call chain**: an operation traversing `bundle.Deploy → ZarfDeployer.DeployBundle → ZarfDeployer.DeployPackage` validates the same config three times. Idempotency keeps this correct, but implementers should keep structural checks cheap and reserve expensive checks (cluster, registry, disk) for the top-most entrypoint in a given call chain.
- **More tests**: every public method gains a nil/invalid-input test path.
- **Slightly more generic error messages**: a first-line `Validate()` call returns `concurrency must be >= 1` rather than `concurrency from config.uds.hcl line 12 must be >= 1`. Callers wrap with their own context.

### Neutral

- **Test fixtures**: tests that hand-craft `UDSBundleConfig{}` literals without `Global` and `Options` populated will fail at the public method's `Validate()` call rather than later with a nil panic. Strictly an improvement, but a handful of fixtures need a shared helper (`newTestConfig()` already exists in `pkg/bundle/create_test.go` and should be promoted).

### Corner cases

- `ConfigResolver.Resolve()` returns a merged `*UDSBundleConfig` and no longer calls `bundle.ValidateConfig`. The first downstream library call's `Validate()` method validates. This eliminates the ambiguity over which layer owns config validation.

## References

- [ADR-0002 - CLI Architecture Patterns](0002-cli-architecture-patterns.md)
- Helm `pkg/action` - Go reference: https://pkg.go.dev/helm.sh/helm/v3/pkg/action
- kubectl `cli-runtime` - Go reference: https://pkg.go.dev/k8s.io/cli-runtime
- Alexis King, *Parse, don't validate* - https://lexi-lambda.github.io/blog/2019/11/05/parse-don-t-validate/
- Go Validation Strategy: Boundaries, Trust, and Defense-in-Depth - https://alnah.io/post/go-validation-strategy/
