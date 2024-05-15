---
title: Overrides
type: docs
weight: 1
---

## Syntax

Consider the following bundle `overrides`:

```yaml
packages:
  - name: helm-overrides-package
    path: "path/to/pkg"
    ref: 0.0.1

    overrides:
      helm-overrides-component: # component name inside of the helm-overrides-package Zarf pkg
        podinfo:                # chart name from the helm-overrides-component component
          values:
            - path: "replicaCount"
              value: 2
          variables:
            - name: UI_COLOR
              path: "ui.color"
              description: "Set the color for podinfo's UI"
              default: "purple"
```

In the above example, the Zarf Package `helm-overrides-package` has a specific component named `helm-overrides-component`, which contains a Helm chart named `podinfo`. Notably, these names serve as keys in the `overrides` block. Within the `podinfo` chart, the `replicaCount` value has been overridden to `2`, and a variable named `UI_COLOR` is specifically overridden to the color `purple`.

## Values

In the `overrides` block, you will find a list of path and value pairs. This feature enables users to override values within the underlying Helm chart of a Zarf Package component.

{{% alert-note %}}
Please note that these values are initially set by the authors of the bundle and, once the bundle has been created, they **cannot be modified**.
{{% /alert-note %}}

### Path

The path employs dot notation to indicate the location of a value intended for overriding within the base Helm chart. For example, in the `podinfo` chart, the `replicaCount` path is situated at the highest level of the [`podinfo values.yaml`](https://github.com/stefanprodan/podinfo/blob/master/charts/podinfo/values.yaml) so the path is expressed as `replicaCount`. Conversely, the `ui.color` path is found beneath the `ui` key, so the designated path is expressed as `ui.color`.

### Value

The `value` represents the content to be set at the specified `path`. It can include straightforward data types like numbers and strings, but it is also versatile enough to handle intricate structures such as lists and objects. For example:

```yaml
...
    overrides:
      helm-overrides-component:
        podinfo:
          values:
            - path: "podinfo.tolerations"
              value:
                - key: "unicorn"
                  operator: "Equal"
                  value: "defense"
                  effect: "NoSchedule"
            - path: podinfo.podAnnotations
              value:
                customAnnotation: "customValue"
```

When using a variable exported from another package, you can use its value to set another variable using the syntax `${...}`. In the following example, the `COLOR` variable is used to establish the value of `podinfo.ui.color`.

```yaml
kind: UDSBundle
metadata:
  name: export-vars
  description: Example for using an imported variable to set an overrides value
  version: 0.0.1

packages:
  - name: output-var
    repository: localhost:888/output-var
    ref: 0.0.1
    exports:
      - name: COLOR

  - name: helm-overrides
    path: "../../packages/helm"
    ref: 0.0.1

    overrides:
      podinfo-component:
        unicorn-podinfo:
          values:
            - path: "podinfo.replicaCount"
              value: 1
            - path: "podinfo.ui.color"
              value: ${COLOR}
```

## Variables

Variables function similarly to values, enabling users to override configurations within a Zarf Package component's underlying Helm chart. They also share a similar syntax. However, unlike values, variables offer the flexibility of being overridden dynamically during deploy time. For example, consider the following `variables`:

```yaml
...
    overrides:
      helm-overrides-component:
        podinfo:
          variables:
           - name: UI_COLOR
             path: "ui.color"
             description: "Set the color for podinfo's UI"
             default: "purple"
```

There are three ways to override the `UI_COLOR` variable:

- **UDS Config:** Create a `uds-config.yaml` file within the bundle's directory and specify the variables you wish to override. To modify the `UI_COLOR` variable, create a `uds-config.yaml`. For example:

```yaml
  variables:
    helm-overrides-package:
      ui_color: green
```

Note that the variable for `UI_COLOR` can be either upper or lower case.

- **Environment Variables:** Create an environment variable prefixed with `UDS_` followed by the variable name. For example, to override the `UI_COLOR` variable, you would create an environment variable named `UDS_UI_COLOR` and assign it the desired value.

Note that the environment variables hold precedence over variables specified in the `uds-config.yaml` configuration file.

- **`--set` Flag:** Override a variable using the UDS CLI's `--set` flag. For example, the override the `UI_COLOR` variable, run one of the following commands:

```cli
# by default ui_color will apply to all packages in the bundle
uds deploy example-bundle --set ui_color=green

# to specify a specific package that the variable should apply to you can prepend th package name to the variable
uds deploy example-bundle --set helm-overrides-package.ui_color=green
```

{{% alert-note %}}
When using Helm override variables and Zarf variables, which share the same `--set` syntax, exercise caution with variable naming to prevent conflicts.
{{% /alert-note %}}

### Variable Precendence

Variable precedence is as follows:

1. Environment variables.
2. `uds-config.yaml` variables.
3. Variables that are `default` in the `uds-bundle.yaml`.

## Namespace

Users can specify a namespace for a packaged Helm chart to be installed in. For instance, to deploy a chart in the `custom-podinfo` namespace, specify the `namespace` in the `overrides` block:

```yaml
kind: UDSBundle
metadata:
   name: example-bundle
   version: 0.0.1

packages:
   - name: helm-overrides-package
     path: "../../packages/helm"
     ref: 0.0.1"
     overrides:
        podinfo-component:
          unicorn-podinfo:
             namespace: custom-podinfo # custom namespace!
             values:
                - path: "podinfo.replicaCount"
                  value: 1
```
