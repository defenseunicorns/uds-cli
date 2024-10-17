---
title: uds monitor
---
## uds monitor

Monitor a UDS Cluster

### Synopsis

Tools for monitoring a UDS Cluster and connecting to the UDS Engine for advanced troubleshooting

### Options

```
  -h, --help               help for monitor
  -n, --namespace string   Limit monitoring to a specific namespace
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
* [uds monitor pepr](/cli/command-reference/uds_monitor_pepr/)	 - Observe Pepr operations in a UDS Cluster
