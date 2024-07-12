---
title: uds dev
description: UDS CLI command reference for <code>uds dev</code>.
type: docs
---
## uds dev

[beta] Commands useful for developing bundles

### Options

```
  -h, --help   help for dev
```

### Options inherited from parent commands

```
  -a, --architecture string   Architecture for UDS bundles and Zarf packages
      --insecure              Allow access to insecure registries and disable other recommended security enforcements such as package checksum and signature validation. This flag should only be used if you have a specific reason and accept the reduced security posture.
  -l, --log-level string      Log level when running UDS-CLI. Valid options are: warn, info, debug, trace (default "info")
      --no-color              Disable color output
      --no-log-file           Disable log file creation
      --no-progress           Disable fancy UI progress bars, spinners, logos, etc
      --oci-concurrency int   Number of concurrent layer operations to perform when interacting with a remote bundle. (default 3)
      --tmpdir string         Specify the temporary directory to use for intermediate files
      --uds-cache string      Specify the location of the UDS cache directory (default "~/.uds-cache")
```

### SEE ALSO

* [uds](/cli/command-reference/uds/)	 - CLI for UDS Bundles
* [uds dev deploy](/cli/command-reference/uds_dev_deploy/)	 - [beta] Creates and deploys a UDS bundle in dev mode

