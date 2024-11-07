---
title: uds
description: UDS CLI command reference for <code>uds</code>.
---
## uds

CLI for UDS Bundles

```
uds COMMAND [flags]
```

### Options

```
  -a, --architecture string   Architecture for UDS bundles and Zarf packages
  -h, --help                  help for uds
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

* [uds completion](/reference/cli/commands/uds_completion/)	 - Generate the autocompletion script for the specified shell
* [uds create](/reference/cli/commands/uds_create/)	 - Create a bundle from a given directory or the current directory
* [uds deploy](/reference/cli/commands/uds_deploy/)	 - Deploy a bundle from a local tarball or oci:// URL
* [uds dev](/reference/cli/commands/uds_dev/)	 - [beta] Commands useful for developing bundles
* [uds inspect](/reference/cli/commands/uds_inspect/)	 - Display the metadata of a bundle
* [uds logs](/reference/cli/commands/uds_logs/)	 - View most recent UDS CLI logs
* [uds monitor](/reference/cli/commands/uds_monitor/)	 - Monitor a UDS Cluster
* [uds publish](/reference/cli/commands/uds_publish/)	 - Publish a bundle from the local file system to a remote registry
* [uds pull](/reference/cli/commands/uds_pull/)	 - Pull a bundle from a remote registry and save to the local file system
* [uds remove](/reference/cli/commands/uds_remove/)	 - Remove a bundle that has been deployed already
* [uds run](/reference/cli/commands/uds_run/)	 - Run a task using maru-runner
* [uds version](/reference/cli/commands/uds_version/)	 - Shows the version of the running UDS-CLI binary

