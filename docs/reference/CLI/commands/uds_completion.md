---
title: uds completion
description: UDS CLI command reference for <code>uds completion</code>.
---
## uds completion

Generate the autocompletion script for the specified shell

### Synopsis

Generate the autocompletion script for uds for the specified shell.
See each sub-command's help for details on how to use the generated script.


### Options

```
  -h, --help   help for completion
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
* [uds completion bash](/reference/cli/commands/uds_completion_bash/)	 - Generate the autocompletion script for bash
* [uds completion fish](/reference/cli/commands/uds_completion_fish/)	 - Generate the autocompletion script for fish
* [uds completion zsh](/reference/cli/commands/uds_completion_zsh/)	 - Generate the autocompletion script for zsh

