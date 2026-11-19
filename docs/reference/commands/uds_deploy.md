---
title: uds deploy
description: UDS CLI command reference for <code>uds deploy</code>.
---
## uds deploy

Deploy a bundle from a local tarball or oci:// URL

```
uds deploy [BUNDLE_TARBALL|OCI_REF] [flags]
```

### Options

```
  -c, --confirm[=true|false]                   Confirms bundle deployment without prompting. ONLY use with bundles you trust
      --force-conflicts[=true|false]           Force Helm to take ownership of conflicting fields during Server-Side Apply operations. Use when external tools (kubectl, HPAs, etc.) have modified resources. Defaults to false.
  -h, --help[=true|false]                      help for deploy
  -p, --packages stringArray                   Specify which zarf packages you would like to deploy from the bundle. By default all zarf packages in the bundle are deployed.
  -r, --resume[=true|false]                    Only deploys packages from the bundle which haven't already been deployed
      --retries int                            Specify the number of retries for package deployments (applies to all pkgs in a bundle) (default 3)
      --set stringToString                     Specify deployment variables to set on the command line (KEY=value) (default [])
      --skip-architecture-check[=true|false]   Skip package-to-cluster architecture compatibility checks. Intended for development and testing; not recommended for production.
```

### Options inherited from parent commands

```
  -a, --architecture string                      Architecture for UDS bundles and Zarf packages
      --features string                          Features, comma separated name, name=true, or name=false pairs. CLI_FEATURES is also supported.
      --insecure[=true|false]                    Allow access to insecure registries and disable other recommended security enforcements such as package checksum and signature validation. This flag should only be used if you have a specific reason and accept the reduced security posture.
  -l, --log-level string                         Log level when running UDS-CLI. Valid options are: warn, info, debug, trace (default "info")
      --no-color[=true|false]                    Disable color output
      --no-log-file[=true|false]                 Disable log file creation
      --no-progress[=true|false]                 Disable fancy UI progress bars, spinners, logos, etc
      --oci-concurrency int                      Number of concurrent layer operations to perform when interacting with a remote bundle. (default 3)
      --skip-signature-validation[=true|false]   Skip signature validation for packages
      --tmpdir string                            Specify the temporary directory to use for intermediate files
      --uds-cache string                         Specify the location of the UDS cache directory (default "~/.uds-cache")
```

### SEE ALSO

* [uds](/reference/commands/uds/)	 - CLI for UDS Bundles

