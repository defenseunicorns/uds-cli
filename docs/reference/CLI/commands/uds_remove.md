---
title: uds remove
description: UDS CLI command reference for <code>uds remove</code>.
---
## uds remove

Remove a bundle that has been deployed already

```
uds remove [BUNDLE_TARBALL|OCI_REF] [flags]
```

### Options

```
  -c, --confirm                REQUIRED. Confirm the removal action to prevent accidental deletions
  -h, --help                   help for remove
  -p, --packages stringArray   Specify which zarf packages you would like to remove from the bundle. By default all zarf packages in the bundle are removed.
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

* [uds](/reference/cli/commands/uds/)	 - CLI for UDS Bundles

