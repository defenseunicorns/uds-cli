---
title: uds dev disassemble
description: UDS CLI command reference for <code>uds dev disassemble</code>.
---
## uds dev disassemble

[beta] Convert a Zarf package into rebuildable offline source

### Synopsis

[beta] Extract a complete Zarf package and rewrite its packaged resources into a local source directory that can be rebuilt offline. Zarf packages are supported today.

```
uds dev disassemble <source> <output-dir> [flags]
```

### Options

```
  -h, --help   help for disassemble
```

### Options inherited from parent commands

```
  -a, --architecture string         Architecture for UDS bundles and Zarf packages
      --features string             Features, comma separated name, name=true, or name=false pairs. CLI_FEATURES is also supported.
      --insecure                    Allow access to insecure registries and disable other recommended security enforcements such as package checksum and signature validation. This flag should only be used if you have a specific reason and accept the reduced security posture.
  -l, --log-level string            Log level when running UDS-CLI. Valid options are: warn, info, debug, trace (default "info")
      --no-color                    Disable color output
      --no-log-file                 Disable log file creation
      --no-progress                 Disable fancy UI progress bars, spinners, logos, etc
      --oci-concurrency int         Number of concurrent layer operations to perform when interacting with a remote bundle. (default 3)
      --skip-signature-validation   Skip signature validation for packages
      --tmpdir string               Specify the temporary directory to use for intermediate files
      --uds-cache string            Specify the location of the UDS cache directory (default "~/.uds-cache")
```

### SEE ALSO

* [uds dev](/reference/commands/uds_dev/)	 - [beta] Commands useful for developing bundles

