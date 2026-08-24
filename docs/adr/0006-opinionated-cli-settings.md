# 6. Opinionated CLI Settings

Date: 2026-02-27

## Status

Accepted

## Context

CLI operations - including but not limited to bundle create, pull, push, deploy - involve several distinct concerns that need user-configurable options. Rather than exposing a large, loosely-defined option surface or simply continuing with the status quo from the old CLI, UDS CLI Next takes an opinionated approach. We will define a minimal set of options motivated by specific, well-understood use cases.

### Use-Cases Requiring Settings

#### Cross-architecture workflows

Engineers building and deploying bundles in CI/CD pipelines or on heterogeneous hardware need to target a CPU architecture different from their local machine (e.g., building `amd64` artifacts on an Apple Silicon laptop, or deploying to an `arm64` cluster from an `amd64` runner). The `architecture` option controls the target arch for create, pull, and deploy operations, defaulting to the local runtime architecture.

#### Resource constraints

Environments such as air-gapped systems, shared build runners, or containers with limited storage may need to redirect cache and temporary storage to specific paths. The `uds_cache` and `tmp_dir` options allow operators to control where UDS CLI stores pulled layers and intermediate working files respectively.

The `concurrency` option lets operators balance throughput against resource usage. It controls the degree of parallelism across all concurrent operations in the CLI, such as OCI layer pulls at create time where a larger CI-based build system is assumed. Higher values reduce overall operation time at the cost of increased network, disk, and memory pressure. Defaults to `10`.

#### Insecure and local registry setups

Developers frequently work against local or private registries that do not use publicly trusted TLS certificates, or that serve over plain HTTP. Two distinct options handle this:
- `plain_http`: allow pulls from and pushes to registries serving over plain HTTP (e.g., a local `registry:2` container without TLS).
- `skip_tls_verify`: skip TLS certificate verification for registries that have self-signed or otherwise untrusted certificates.

Separating these two options makes the security trade-off explicit at the point of use.

#### Logging verbosity

The `log_level` option controls the verbosity of UDS CLI's structured log output globally across all commands. See [ADR-0004](0004-logging-and-output-strategy.md) for the full treatment of log level values, format, and destination.

## Decision

### Exposed Options

UDS CLI Next exposes the following options:

| Option | Component | CLI Flag |
|--------|-----------|----------|
| `log_level` | Global | `--log-level` |
| `architecture` | Bundle | `--architecture` |
| `plain_http` | Bundle | `--plain-http` |
| `skip_tls_verify` | Bundle | `--skip-tls-verify` |
| `uds_cache` | Bundle | `--uds-cache` |
| `tmp_dir` | Bundle | `--tmp-dir` |
| `concurrency` | Bundle | `--concurrency` |

In the `config.uds.hcl` file these would look like:

Log level can be set via the `--log-level` CLI flag on the root command or via the `log_level` option in the config file. CLI flags always take precedence over config file values. See [ADR-0004](0004-logging-and-output-strategy.md) for details on the logging strategy.

```hcl
options {
  log_level          = "info"
  architecture       = "amd64"
  plain_http         = false
  skip_tls_verify    = false
  uds_cache          = "/tmp/uds-cache"
  tmp_dir            = "/tmp/uds-tmp"
  concurrency        = 10
}
```

Options can be set by either a config file or explicit CLI flags on a command invocation. CLI flags always take precedence over config file values.

### Progress Output

The old UDS CLI exposed a `--no-progress` flag to suppress interactive progress bars and spinners. UDS CLI Next intentionally omits this flag. Instead of defaulting to progress-on and requiring users to opt out, UDS CLI Next defaults to **no progress output** and only enables interactive elements (spinners, progress bars) when conditions are met:

1. **TTY detection (primary)**: Interactive progress is only enabled when stdout is a TTY. When output is piped, redirected to a file, or running in a non-interactive shell, progress elements remain off by default - no flag required.
2. **Environment signal safeguards**: Even when a TTY is detected, the CLI respects well-known environment signals - `CI=true`, `NO_COLOR`, `TERM=dumb`, and similar conventions - to keep progress disabled. This handles cases where CI systems or automation frameworks allocate a pseudo-TTY but interactive output is still unwanted.

This approach eliminates a flag that users had to remember to set and instead does the right thing automatically. See [ADR-0005](0005-interactivity.md) for the full treatment of interactive vs. non-interactive behavior.

## Consequences

### Positive

- **Use-case driven**: Every option maps to a concrete, documented scenario, making it easier to understand and justify the option set.
- **Decoupled from Zarf's surface**: UDS CLI owns its option set; Zarf options can be surfaced to the end user or hidden as use-cases demand.
- **No `no_progress` flag**: Progress output is handled automatically via TTY detection and environment signals rather than a user-managed flag, reducing configuration burden and eliminating a common source of noisy CI logs.
- **Explicit security trade-offs**: Splitting `plain_http` and `skip_tls_verify` into separate flags forces users to acknowledge the specific risk they are accepting, rather than hiding both behind a single `insecure` toggle.

### Negative

- **Limited escape hatch**: Users who need Zarf-specific behavior not covered by this option set have no direct passthrough today. This is intentional as UDS CLI should be an opinionated subset of options.
- **Migration**: Users coming from the old CLI with different flags (e.g., a single `insecure` flag) will need to adjust to the new flags.
- **No per-operation concurrency tuning**: A single `concurrency` option applies uniformly across all concurrent operations. Users cannot, for example, set high concurrency for OCI pulls while keeping other operations sequential. Per-operation flags can be added later if a concrete use case emerges.

### Neutral

- **Config file and CLI flags**: This ADR provides support for both CLI flags and a config file, which increases maintenance/paths for configuring the tool, but also increases flexibility for end users.
