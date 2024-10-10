---
title: uds create
---
## uds create

Create a bundle from a given directory or the current directory

```
uds create [DIRECTORY] [flags]
```

### Options

```
  -c, --confirm                       Confirm bundle creation without prompting
  -h, --help                          help for create
  -n, --name string                   Specify the name of the bundle
  -o, --output string                 Specify the output (an oci:// URL) for the created bundle
  -k, --signing-key string            Path to private key file for signing bundles
  -p, --signing-key-password string   Password to the private key file used for signing bundles
  -v, --version string                Specify the version of the bundle
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
