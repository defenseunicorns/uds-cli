---
title: uds inspect
---
## uds inspect

Display the metadata of a bundle

```
uds inspect [BUNDLE_TARBALL|OCI_REF|BUNDLE_YAML_FILE] [flags]
```

### Options

```
  -e, --extract          Create a folder of SBOMs contained in the bundle
  -h, --help             help for inspect
  -k, --key string       Path to a public key file that will be used to validate a signed bundle
  -i, --list-images      Derive images from a uds-bundle.yaml file and list them
  -v, --list-variables   List all configurable variables in a bundle (including zarf variables)
  -s, --sbom             Create a tarball of SBOMs contained in the bundle
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
