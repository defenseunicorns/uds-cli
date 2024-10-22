---
title: uds monitor pepr
description: UDS CLI command reference for <code>uds monitor pepr</code>.
---
## uds monitor pepr

Observe Pepr operations in a UDS Cluster

### Synopsis

View UDS Policy enforcements, UDS Operator events and additional Pepr operations

```
uds monitor pepr [policies | operator | allowed | denied | failed | mutated] [flags]
```

### Examples

```

  # Aggregates all admission and operator logs into a single stream
  uds monitor pepr

  # Stream UDS Operator actions (Package processing, status updates, and errors)
  uds monitor pepr operator

  # Stream UDS Policy logs (Allow, Deny, Mutate)
  uds monitor pepr policies

  # Stream UDS Policy allow logs
  uds monitor pepr allowed

  # Stream UDS Policy deny logs
  uds monitor pepr denied

  # Stream UDS Policy mutation logs
  uds monitor pepr mutated

  # Stream UDS Policy deny logs and UDS Operator error logs
  uds monitor pepr failed
```

### Options

```
  -f, --follow           Continuously stream Pepr logs
  -h, --help             help for pepr
      --json             Return the raw JSON output of the logs
      --since duration   Only return logs newer than a relative duration like 5s, 2m, or 3h. Defaults to all logs.
  -t, --timestamps       Show timestamps in Pepr logs
```

### Options inherited from parent commands

```
  -a, --architecture string   Architecture for UDS bundles and Zarf packages
      --insecure              Allow access to insecure registries and disable other recommended security enforcements such as package checksum and signature validation. This flag should only be used if you have a specific reason and accept the reduced security posture.
  -l, --log-level string      Log level when running UDS-CLI. Valid options are: warn, info, debug, trace (default "info")
  -n, --namespace string      Limit monitoring to a specific namespace
      --no-color              Disable color output
      --no-log-file           Disable log file creation
      --no-progress           Disable fancy UI progress bars, spinners, logos, etc
      --oci-concurrency int   Number of concurrent layer operations to perform when interacting with a remote bundle. (default 3)
      --tmpdir string         Specify the temporary directory to use for intermediate files
      --uds-cache string      Specify the location of the UDS cache directory (default "~/.uds-cache")
```

### SEE ALSO

* [uds monitor](/reference/cli/commands/uds_monitor/)	 - Monitor a UDS Cluster

