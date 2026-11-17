# 29. Zarf init options

Date: 2026-11-16

## Status

Accepted

## Context

Users need to configure external registries and Git servers, registry mode, and other Zarf initialization settings through CLI Next.
These settings cannot be supplied through package values overrides alone, even with a separate init package: Zarf needs them to select bootstrap behavior, push destinations, and credentials independently of chart configuration.
Settings already exposed by the package, such as storage class, can use existing package inputs.

Legacy CLI supports these settings today through `UDS_INIT_*` environment variables and reserved `INIT_*` variables in `uds-config.yaml`.
It translates them into Zarf deployment options when deploying an init package.
CLI Next has no equivalent mechanism, leaving a gap for operators migrating from Legacy.

## Decision

Add optional `options.zarf_init` to consumer-owned `config.uds.hcl`, extending [ADR-0006](0006-opinionated-cli-settings.md).
The proposed syntax is:

```hcl
options {
  zarf_init {
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

### Deployment behavior

Apply options only when the verified package reports `IsInitConfig()`.
Leave omitted options unset.
Zarf preserves existing mutation policy or defaults to `all` when absent; remove Next's hardcoded `all` without adding a state lookup.
Configure mutation policy through init options; the standard beta init values cannot set it.
Settings exposed through package values remain there rather than becoming init options, such as registry and Git server storage classes.

## Consequences

- All init packages in a deployment share the same settings, preventing different settings per package.
- Users must understand which settings belong in `options.zarf_init` and which belong in package variables or values.
- CLI users must supply a `config.uds.hcl` file to customize init options rather than setting them directly through flags.
- Zarf init credentials are not able to be reconciled from `config.uds.hcl`; update existing credentials through the Zarf CLI with `uds tools zarf tools update-creds`.

## Alternatives considered

- **Reserved variables:** retain Legacy's implicit special meanings.
- **Package values alone:** support chart customization, but not all bootstrap operations or saved state.
- **Package-level configuration:** adds flexibility beyond the invocation-wide scope chosen here.

## References

- [Legacy init customization](../how-to-guides/customize-init.mdx).
- Zarf [init options and validation](https://github.com/zarf-dev/zarf/blob/v1.0.1/src/cmd/initialize.go) and [mutation-policy fix](https://github.com/zarf-dev/zarf/pull/5644).
- [Alpha-to-beta init input migration](https://github.com/zarf-dev/zarf/blob/v1.0.0/packages/init/v1beta1/MIGRATION.md) and [Zarf 1.0 migration guidance](https://www.docs.zarf.dev/v1.0).
