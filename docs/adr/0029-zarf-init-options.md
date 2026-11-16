# 29. Zarf init options

Date: 2026-11-16

## Status

Accepted

## Context

CLI Next needs external registry and Git configuration, registry mode selection, and init storage customization.
Package values handle storage, but cannot supply all Zarf bootstrap settings.
Legacy uses reserved `INIT_*` variables; Next needs explicit configuration.

## Decision

Add optional `options.init` to consumer-owned `config.uds.hcl`, extending
[ADR-0006](0006-opinionated-cli-settings.md). The proposed syntax is:

```hcl
options {
  init {
    registry_mode          = "external"
    registry_url           = "registry.example.com:5000"
    registry_push_username = "zarf-push"
    registry_push_password = file("secrets/registry-password.txt")
    git_url                = "https://git.example.com"
    git_push_username      = "zarf-push"
    git_push_password      = file("secrets/git-token.txt")
  }
}
```

### Supported inputs

| Group | Inputs |
| --- | --- |
| Registry | `registry_url`, `registry_mode`, `registry_port`, `registry_secret`, `registry_push_username`, `registry_push_password`, `registry_pull_username`, `registry_pull_password` |
| Git | `git_url`, `git_push_username`, `git_push_password`, `git_pull_username`, `git_pull_password` |
| Bootstrap | `injector_image`, `injector_port` |
| Agent | `agent_mutation_policy`, `agent_tls_ca`, `agent_tls_cert`, `agent_tls_key` |

Deprecated inputs are not supported as they have been removed as of Zarf v1.0

### Storage customization

Use Next variables for alpha init packages and package `values_files` for beta:

| Package format | Registry storage class | Git server storage class |
| --- | --- | --- |
| v1alpha1 variables | `REGISTRY_STORAGE_CLASS` | `GIT_SERVER_STORAGE_CLASS` |
| v1beta1 values | `registry.persistence.storageClass` | `gitServer.persistence.storageClass` |


### Deployment behavior

Apply options only when the verified package reports `IsInitConfig()`.
Redact secrets from logs, previews, and persisted results.

Leave omitted options unset.
Zarf preserves existing mutation policy or defaults to `all` when absent; remove Next's hardcoded `all` without adding a state lookup.
Explicit changes follow Zarf's update restrictions.
Configure mutation policy through init options; the standard beta init values cannot set it.

## Limitations

Options apply to every selected init package.
Different settings require separate invocations and config files; package-specific overrides are unsupported.

## Consequences

Initialization becomes configurable without rebuilding bundles.
Maintain validation, redaction, and alpha/beta compatibility.
Test fresh init, policy preservation, explicit overrides, and regular packages receiving no init options.

## Alternatives considered

- **Reserved variables:** retain Legacy's implicit special meanings.
- **Package values alone:** support chart customization, but not all bootstrap
  operations or saved state.
- **Package-level configuration:** adds flexibility beyond the invocation-wide
  scope chosen here.

## References

- [Legacy init customization](../how-to-guides/customize-init.mdx).
- Zarf [init options and validation](https://github.com/zarf-dev/zarf/blob/v1.0.1/src/cmd/initialize.go)
  and [mutation-policy fix](https://github.com/zarf-dev/zarf/pull/5644).
- [Alpha-to-beta init input migration](https://github.com/zarf-dev/zarf/blob/v1.0.0/packages/init/v1beta1/MIGRATION.md)
  and [Zarf 1.0 migration guidance](https://www.docs.zarf.dev/v1.0).
