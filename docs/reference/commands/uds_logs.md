---
title: uds logs
description: UDS CLI command reference for <code>uds logs</code>.
---
## uds logs

View most recent UDS CLI logs

```
uds logs [flags]
```

### Options

```
  -h, --help[=true|false]   help for logs
```

### Options inherited from parent commands

```
  -a, --architecture string                      Architecture for UDS bundles and Zarf packages
      --features string                          Features, comma separated name, name=true, or name=false pairs. CLI_FEATURES is also supported.
      --insecure[=true|false]                    Allow access to insecure registries and disable other recommended security enforcements such as package checksum and signature validation. This flag should only be used if you have a specific reason and accept the reduced security posture.
  -l, --log-level string                         Log level when running UDS-CLI. Valid options are: warn, info, debug, trace (default "info")
      --no-color[=true|false]                    Disable color output
      --no-log-file[=true|false]                 Disable log file creation
      --no-progress[=true|false]                 Disable fancy UI progress bars, spinners, logos, etc
      --oci-concurrency int                      Number of concurrent layer operations to perform when interacting with a remote bundle. (default 3)
      --skip-signature-validation[=true|false]   Skip signature validation for packages
      --tmpdir string                            Specify the temporary directory to use for intermediate files
      --uds-cache string                         Specify the location of the UDS cache directory (default "~/.uds-cache")
```

### SEE ALSO

* [uds](/reference/commands/uds/)	 - CLI for UDS Bundles

