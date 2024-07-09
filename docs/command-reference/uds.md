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
      --no-log-file           Disable log file creation
      --no-progress           Disable fancy UI progress bars, spinners, logos, etc
      --oci-concurrency int   Number of concurrent layer operations to perform when interacting with a remote bundle. (default 3)
      --tmpdir string         Specify the temporary directory to use for intermediate files
      --uds-cache string      Specify the location of the UDS cache directory (default "~/.uds-cache")
```

### SEE ALSO

* [uds completion](uds_completion.md)	 - Generate the autocompletion script for the specified shell
* [uds create](uds_create.md)	 - Create a bundle from a given directory or the current directory
* [uds deploy](uds_deploy.md)	 - Deploy a bundle from a local tarball or oci:// URL
* [uds dev](uds_dev.md)	 - [beta] Commands useful for developing bundles
* [uds inspect](uds_inspect.md)	 - Display the metadata of a bundle
* [uds logs](uds_logs.md)	 - View most recent UDS CLI logs
* [uds monitor](uds_monitor.md)	 - Monitor a UDS Cluster
* [uds publish](uds_publish.md)	 - Publish a bundle from the local file system to a remote registry
* [uds pull](uds_pull.md)	 - Pull a bundle from a remote registry and save to the local file system
* [uds remove](uds_remove.md)	 - Remove a bundle that has been deployed already
* [uds run](uds_run.md)	 - Run a task using maru-runner
* [uds version](uds_version.md)	 - Shows the version of the running UDS-CLI binary

