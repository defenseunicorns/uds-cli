---
title: uds dev tofu-deploy
description: UDS CLI command reference for <code>uds dev tofu-deploy</code>.
---
## uds dev tofu-deploy

Deploy a bundle from a local tarball or oci:// URL

```
uds dev tofu-deploy [BUNDLE_TARBALL|OCI_REF] [flags]
```

### Options

```
  -h, --help              help for tofu-deploy
      --tf-state string   Path to TF statefile (default "terraform.tfstate")
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

