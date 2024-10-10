---
title: uds
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

* [uds completion](/cli/command-reference/uds_completion/)	 - Generate the autocompletion script for the specified shell
* [uds create](/cli/command-reference/uds_create/)	 - Create a bundle from a given directory or the current directory
* [uds deploy](/cli/command-reference/uds_deploy/)	 - Deploy a bundle from a local tarball or oci:// URL
* [uds dev](/cli/command-reference/uds_dev/)	 - [beta] Commands useful for developing bundles
* [uds inspect](/cli/command-reference/uds_inspect/)	 - Display the metadata of a bundle
* [uds logs](/cli/command-reference/uds_logs/)	 - View most recent UDS CLI logs
* [uds monitor](/cli/command-reference/uds_monitor/)	 - Monitor a UDS Cluster
* [uds publish](/cli/command-reference/uds_publish/)	 - Publish a bundle from the local file system to a remote registry
* [uds pull](/cli/command-reference/uds_pull/)	 - Pull a bundle from a remote registry and save to the local file system
* [uds remove](/cli/command-reference/uds_remove/)	 - Remove a bundle that has been deployed already
* [uds run](/cli/command-reference/uds_run/)	 - Run a task using maru-runner
* [uds ui](/cli/command-reference/uds_ui/)	 - [beta] Launch UDS Runtime and view UI
* [uds version](/cli/command-reference/uds_version/)	 - Shows the version of the running UDS-CLI binary
