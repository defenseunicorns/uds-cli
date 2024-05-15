---
title: CLI Variables
type: docs
weight: 3
---

## Importing and Exporting Variables

Zarf Package variables can be passed between Zarf Packages, see the example below:

```yaml
kind: UDSBundle
metadata:
  name: simple-vars
  description: show how vars work
  version: 0.0.1

packages:
  - name: output-var
    repository: localhost:888/output-var
    ref: 0.0.1
    exports:
      - name: OUTPUT
  - name: receive-var
    repository: localhost:888/receive-var
    ref: 0.0.1
    imports:
      - name: OUTPUT
        package: output-var
```

In Zarf Packages, variables intended for accessibility by other packages are situated within the `export` block of the respective Zarf Package. By default, all variables marked for `export` are globally accessible to all packages within a bundle. However, when dealing with potential variable name collisions or when a specific exported variable needs to be consumed by another package, the `imports` key can be used. This key facilitates the explicit association of both the variable name and the package from which it is exported.

In the provided example, the `OUTPUT` variable is generated as part of a Zarf Action within the `output-var` package. The `receive-var` package, in turn, anticipates the availability of a variable named `OUTPUT` and explicitly imports it using the `imports` key.

## Sharing Variables

To streamline the configuration of Zarf Variables across multiple packages within a bundle, consider employing environment variables prefixed with `UDS_`. This approach eliminates the need to repetitively set variables using import/export syntax. For instance, if several packages share a common variable, such as `DOMAIN`, you can set it once using a `UDS_DOMAIN` environment variable, and it will automatically apply to all packages in the bundle. Alternatively, a similar configuration can be achieved using the `shared` key in the `uds-config.yaml` file.

On deploy, package variables can also be set using the `--set` flag. When the package name is excluded from the key, for example: `--set super=true`, the variable is applied to all packages. If the package name is included in the key, for example: `--set cool-package.super=true`, the variable exclusively applies to the specified package. This provides a flexible mechanism for targeted configuration adjustments during deployment.

## Variable Precedence and Specificity

Within a bundle, variables can originate from four distinct sources, each with a specified order of precedence ranging from the least to the most specific. These sources are outlined below:

- Variables declared in a Zarf Package.
- Variables that have been `import` from a bundles package's `export`.
- Varaibles configured in the `shared` key in a `uds-config.yaml`.
- Variables configured in the `variables` key in a `uds-config.yaml`.
- Variables set with an environment variable prefixed with `UDS_`, for example: `UDS_OUTPUT`.
- Variables set using the `--set` flag when executing the `uds deploy` command.

{{% alert-note %}}
Variables established using the `--set` flag take precedence over variables from all other outlined sources.
{{% /alert-note %}}
