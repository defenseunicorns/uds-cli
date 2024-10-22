---
title: uds dev deploy
description: UDS CLI command reference for <code>uds dev deploy</code>.
---
## uds dev deploy

[beta] Creates and deploys a UDS bundle in dev mode

### Synopsis

[beta] Creates and deploys a UDS bundle from a given directory or OCI repository in dev mode, setting package options like YOLO mode for faster iteration.

```
uds dev deploy [BUNDLE_DIR|OCI_REF] [flags]
```

### Options

```
  -f, --flavor string          [beta] Specify which zarf package flavor you want to use.
      --force-create           [beta] For local bundles with local packages, specify whether to create a zarf package even if it already exists.
  -h, --help                   help for deploy
  -p, --packages stringArray   Specify which zarf packages you would like to deploy from the bundle. By default all zarf packages in the bundle are deployed.
  -r, --ref stringToString     Specify which zarf package ref you want to deploy. By default the ref set in the bundle yaml is used. (default [])
      --set stringToString     Specify deployment variables to set on the command line (KEY=value) (default [])
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

* [uds dev](/reference/cli/commands/uds_dev/)	 - [beta] Commands useful for developing bundles

